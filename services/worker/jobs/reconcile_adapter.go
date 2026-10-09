package jobs

import (
	"context"
	"fmt"

	"github.com/aspirit/xsocial/internal/domains/post"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/otelutil"
)

// likeCountSource adapts the repositories (and the cache) to LikeCountSource.
type likeCountSource struct {
	posts interfaces.IPostRepository
	likes interfaces.IPostLikeRepository
	cache cache.Cache // optional; nil disables invalidation
}

// NewLikeCountSource builds a LikeCountSource over the post and like repositories.
func NewLikeCountSource(posts interfaces.IPostRepository, likes interfaces.IPostLikeRepository, c cache.Cache) LikeCountSource {
	return &likeCountSource{posts: posts, likes: likes, cache: c}
}

func (s *likeCountSource) NextPostIDs(ctx context.Context, afterID uint64, limit int64) ([]uint64, error) {
	ids, err := s.posts.ScanIDs(ctx, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("scan post ids: %w", err)
	}

	return ids, nil
}

func (s *likeCountSource) ExpectedLikeCounts(ctx context.Context, ids []uint64) (map[uint64]int64, error) {
	counts, err := s.likes.CountByPosts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("count likes: %w", err)
	}

	return counts, nil
}

func (s *likeCountSource) StoredLikeCounters(ctx context.Context, ids []uint64) (map[uint64]model.LikeCounter, error) {
	counters, err := s.posts.LikeCounters(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("stored like counters: %w", err)
	}

	return counters, nil
}

// CorrectLikeCount applies the versioned compare-and-set and, when it took
// effect, drops the cached post so readers do not keep serving the stale counter.
func (s *likeCountSource) CorrectLikeCount(ctx context.Context, id uint64, version, want int64) (bool, error) {
	ok, err := s.posts.CorrectLikeCount(ctx, id, version, want)
	if err != nil {
		return false, fmt.Errorf("correct like count: %w", err)
	}
	if ok {
		cache.Invalidate(ctx, s.cache, post.CacheKey(id))
	}

	return ok, nil
}

// OtelDriftRecorder records reconciliation drift as a counter metric, so drift
// is alertable on /metrics instead of buried in logs.
type OtelDriftRecorder struct {
	provider   *otelutil.OtelProvider
	metricName string
}

// NewOtelDriftRecorder builds a DriftRecorder backed by OpenTelemetry. A nil
// provider makes it a no-op.
func NewOtelDriftRecorder(provider *otelutil.OtelProvider) *OtelDriftRecorder {
	return &OtelDriftRecorder{provider: provider, metricName: "reconciliation_like_count_drift_total"}
}

// RecordDrift implements DriftRecorder.
func (o *OtelDriftRecorder) RecordDrift(ctx context.Context, drift int64) {
	if o.provider == nil {
		return
	}
	o.provider.RecordCounter(ctx, o.metricName, drift)
}
