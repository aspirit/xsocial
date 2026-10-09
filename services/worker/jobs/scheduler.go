// Package jobs provides the worker's scheduled background jobs (reconciliation,
// cleanup, etc.) and a lightweight interval scheduler with no external deps.
package jobs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/aspirit/xsocial/pkg/logger"
)

// Job is a unit of recurring work run by the Scheduler.
type Job interface {
	// Name identifies the job in logs, metrics and leases.
	Name() string
	// Run executes one iteration. It should honor ctx cancellation.
	Run(ctx context.Context) error
}

// Leaser hands out per-job leases so that N worker replicas run a job about
// once per interval instead of N times (interfaces.IJobLeaseRepository
// satisfies it).
type Leaser interface {
	TryAcquire(ctx context.Context, name, holder string, ttl time.Duration) (bool, error)
}

type scheduledJob struct {
	job      Job
	interval time.Duration
}

// Scheduler runs registered jobs on fixed intervals using time.Ticker. It runs
// each job once at startup and then every interval, until the context is done.
type Scheduler struct {
	log    *logger.Logger
	jobs   []scheduledJob
	leaser Leaser
	holder string
}

// NewScheduler creates a Scheduler.
func NewScheduler(log *logger.Logger) *Scheduler {
	return &Scheduler{log: log.Named("scheduler")}
}

// WithLeases makes every run acquire the job's lease first (TTL = the job's
// interval) and renew it while the job runs. The lease is deliberately not
// released afterwards: holding it until it lapses is what limits the cluster to
// about one run per interval. What this does and does not guarantee:
//
//   - A lease store error skips the run (fail-closed): the jobs need the same
//     database anyway, and the next interval retries.
//   - A run that outlives the TTL keeps its lease through the heartbeat; if a
//     renewal fails the job's context is cancelled, so it stops at its next
//     checkpoint while another replica may already have taken over.
//   - Overlap is therefore rare but possible (a crashed holder's lease lapses
//     after one interval; a cancelled job may have one write in flight). Every
//     job must tolerate a concurrent runner — the reconciler does, through its
//     versioned compare-and-set.
func (s *Scheduler) WithLeases(l Leaser, holder string) *Scheduler {
	s.leaser = l
	s.holder = holder

	return s
}

// HolderID identifies this process as a lease holder.
func HolderID() string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}

	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// Register adds a job to run every interval. A non-positive interval disables it.
func (s *Scheduler) Register(job Job, interval time.Duration) {
	if interval <= 0 {
		s.log.Warn("job registered with non-positive interval, skipping", zap.String("job", job.Name()))

		return
	}
	s.jobs = append(s.jobs, scheduledJob{job: job, interval: interval})
}

// Run starts every registered job and blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, sj := range s.jobs {
		wg.Add(1)
		go func(sj scheduledJob) {
			defer wg.Done()
			s.runLoop(ctx, sj)
		}(sj)
	}
	wg.Wait()
}

func (s *Scheduler) runLoop(ctx context.Context, sj scheduledJob) {
	// Run once at startup so drift is corrected promptly after a deploy.
	s.runOnce(ctx, sj)

	ticker := time.NewTicker(sj.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx, sj)
		}
	}
}

// runOnce executes a job, recovering from panics so one bad run never takes down
// the scheduler.
func (s *Scheduler) runOnce(ctx context.Context, sj scheduledJob) {
	job := sj.job
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("job panicked", zap.String("job", job.Name()), zap.Any("panic", r))
		}
	}()
	if ctx.Err() != nil {
		return
	}
	if !s.acquire(ctx, sj) {
		return
	}
	runCtx, stopHeartbeat := s.heartbeat(ctx, sj)
	defer stopHeartbeat()
	start := time.Now()
	if err := job.Run(runCtx); err != nil {
		s.log.Error("job failed", zap.String("job", job.Name()), zap.Error(err), zap.Duration("took", time.Since(start)))

		return
	}
	s.log.Info("job completed", zap.String("job", job.Name()), zap.Duration("took", time.Since(start)))
}

// acquire reports whether this replica may run the job now.
func (s *Scheduler) acquire(ctx context.Context, sj scheduledJob) bool {
	if s.leaser == nil {
		return true
	}
	ok, err := s.leaser.TryAcquire(ctx, sj.job.Name(), s.holder, sj.interval)
	if err != nil {
		s.log.Warn("lease store unavailable, skipping run", zap.String("job", sj.job.Name()), zap.Error(err))

		return false
	}
	if !ok {
		s.log.Debug("lease held by another replica, skipping run", zap.String("job", sj.job.Name()))
	}

	return ok
}

// heartbeat renews the job's lease every third of its TTL while the job runs
// and cancels the returned context as soon as a renewal is refused or fails.
func (s *Scheduler) heartbeat(ctx context.Context, sj scheduledJob) (context.Context, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(ctx)
	if s.leaser == nil {
		return runCtx, cancel
	}
	every := sj.interval / 3
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				ok, err := s.leaser.TryAcquire(runCtx, sj.job.Name(), s.holder, sj.interval)
				if err != nil || !ok {
					s.log.Warn("lease lost while running, cancelling job", zap.String("job", sj.job.Name()), zap.Error(err))
					cancel()

					return
				}
			}
		}
	}()

	return runCtx, cancel
}
