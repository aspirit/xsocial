package jobs

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/pkg/logger"
)

// LikeCountSource supplies the data needed to reconcile the denormalised
// post.like_count against the source of truth (the post_like collection).
// All methods work on a page of IDs so one run costs O(posts / pageSize)
// round-trips instead of one per post.
type LikeCountSource interface {
	// NextPostIDs returns up to limit post IDs with id > afterID, ascending.
	// An empty result signals the end of the scan.
	NextPostIDs(ctx context.Context, afterID uint64, limit int64) ([]uint64, error)
	// ExpectedLikeCounts returns the true counts (aggregated from post_like).
	ExpectedLikeCounts(ctx context.Context, postIDs []uint64) (map[uint64]int64, error)
	// StoredLikeCounters returns the counters currently stored on the posts, with
	// the version that changes on every write to them.
	StoredLikeCounters(ctx context.Context, postIDs []uint64) (map[uint64]model.LikeCounter, error)
	// CorrectLikeCount sets the counter to want only while its version is still
	// version (nothing wrote it since the read) and reports whether it did.
	CorrectLikeCount(ctx context.Context, postID uint64, version, want int64) (bool, error)
}

// DriftRecorder records reconciliation drift for observability.
type DriftRecorder interface {
	RecordDrift(ctx context.Context, drift int64)
}

// NoopDriftRecorder discards drift (used when telemetry is unavailable).
type NoopDriftRecorder struct{}

// RecordDrift implements DriftRecorder.
func (NoopDriftRecorder) RecordDrift(context.Context, int64) {}

const defaultReconcilePageSize = 500

// LikeCountReconciler periodically recomputes post.like_count from post_like and
// repairs drift (e.g. a crash between two writes on a deployment without
// transactions). It is idempotent: a second run after a correcting run finds no
// drift. With correct=false it only reports (dry-run).
//
// It never fights live traffic: a correction is a compare-and-set against the
// counter VERSION it read (every like and unlike bumps it), so any write that
// lands between the read and the correction makes the correction a no-op — even
// a like followed by an unlike, which leaves the value unchanged but not the
// version. The next run sees consistent data again.
type LikeCountReconciler struct {
	source   LikeCountSource
	metrics  DriftRecorder
	log      *logger.Logger
	pageSize int64
	correct  bool
}

// NewLikeCountReconciler creates a reconciler.
func NewLikeCountReconciler(source LikeCountSource, metrics DriftRecorder, log *logger.Logger, correct bool) *LikeCountReconciler {
	if metrics == nil {
		metrics = NoopDriftRecorder{}
	}

	return &LikeCountReconciler{
		source:   source,
		metrics:  metrics,
		log:      log.Named("reconcile-likecount"),
		pageSize: defaultReconcilePageSize,
		correct:  correct,
	}
}

// Name implements Job.
func (r *LikeCountReconciler) Name() string { return "post_like_count_reconcile" }

// Run implements Job: scans all posts page by page and reconciles like_count.
func (r *LikeCountReconciler) Run(ctx context.Context) error {
	var afterID uint64
	var checked, drifted int64
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("reconcile cancelled: %w", err)
		}
		ids, err := r.source.NextPostIDs(ctx, afterID, r.pageSize)
		if err != nil {
			return fmt.Errorf("list posts (after %d): %w", afterID, err)
		}
		if len(ids) == 0 {
			break
		}
		afterID = ids[len(ids)-1]
		checked += int64(len(ids))

		n, err := r.reconcilePage(ctx, ids)
		if err != nil {
			r.log.Warn("reconcile page failed", zap.Uint64("after", afterID), zap.Error(err))

			continue
		}
		drifted += n
	}
	r.log.Info("reconciliation complete",
		zap.Int64("checked", checked), zap.Int64("drifted", drifted), zap.Bool("corrected", r.correct))

	return nil
}

// reconcilePage reconciles one page and returns how many posts had drift.
func (r *LikeCountReconciler) reconcilePage(ctx context.Context, ids []uint64) (int64, error) {
	// Read the counters (and their versions) BEFORE the relations. Whatever
	// commits in between bumps a version, and the compare-and-set below refuses
	// to write over it. The other order would let a stale aggregate overwrite a
	// fresh counter.
	stored, err := r.source.StoredLikeCounters(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("stored counters: %w", err)
	}
	expected, err := r.source.ExpectedLikeCounts(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("expected counts: %w", err)
	}

	var drifted int64
	for _, id := range ids {
		want, have := expected[id], stored[id].Count
		if want == have {
			continue
		}
		drifted++
		diff := want - have
		if diff < 0 {
			diff = -diff
		}
		r.metrics.RecordDrift(ctx, diff)
		r.log.Warn("like_count drift detected",
			zap.Uint64("post", id), zap.Int64("stored", have), zap.Int64("expected", want))

		if !r.correct {
			continue
		}
		corrected, err := r.source.CorrectLikeCount(ctx, id, stored[id].Version, want)
		if err != nil {
			r.log.Error("failed to correct like_count", zap.Uint64("post", id), zap.Error(err))

			continue
		}
		if !corrected {
			r.log.Info("like_count changed concurrently, left for the next run", zap.Uint64("post", id))
		}
	}

	return drifted, nil
}
