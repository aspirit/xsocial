package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aspirit/xsocial/pkg/logger"
)

type signalJob struct {
	name     string
	runs     int64
	ch       chan struct{}
	panicNow bool
}

func (j *signalJob) Name() string { return j.name }

func (j *signalJob) Run(context.Context) error {
	atomic.AddInt64(&j.runs, 1)
	if j.panicNow {
		panic("boom")
	}
	select {
	case j.ch <- struct{}{}:
	default:
	}

	return nil
}

func TestScheduler_RunsOnStartAndStopsOnCancel(t *testing.T) {
	job := &signalJob{name: "t", ch: make(chan struct{}, 1)}
	s := NewScheduler(logger.NewNopLogger())
	s.Register(job, time.Hour) // long interval: only the startup run should fire

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	select {
	case <-job.ch:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run at startup")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after cancel")
	}
}

func TestScheduler_RecoversFromPanic(t *testing.T) {
	job := &signalJob{name: "panic", ch: make(chan struct{}, 1), panicNow: true}
	s := NewScheduler(logger.NewNopLogger())
	s.Register(job, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	time.Sleep(50 * time.Millisecond) // allow the startup run to panic and recover
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not survive panic or stop")
	}
	if atomic.LoadInt64(&job.runs) == 0 {
		t.Fatal("panicking job never ran")
	}
}

func TestScheduler_SkipsNonPositiveInterval(t *testing.T) {
	s := NewScheduler(logger.NewNopLogger())
	s.Register(&signalJob{name: "disabled", ch: make(chan struct{}, 1)}, 0)
	if len(s.jobs) != 0 {
		t.Fatalf("job with non-positive interval should be skipped, got %d", len(s.jobs))
	}
}

type fakeLeaser struct {
	mu     sync.Mutex
	grants []bool // answers in order; the last one repeats
	err    error
	calls  int
}

func (l *fakeLeaser) TryAcquire(context.Context, string, string, time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.err != nil {
		return false, l.err
	}
	i := l.calls - 1
	if i >= len(l.grants) {
		i = len(l.grants) - 1
	}

	return l.grants[i], nil
}

func (l *fakeLeaser) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.calls
}

func runStartupOnly(t *testing.T, s *Scheduler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond) // let the startup run happen (or be skipped)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after cancel")
	}
}

func TestScheduler_RunsOnlyWithLease(t *testing.T) {
	for _, grant := range []bool{true, false} {
		job := &signalJob{name: "leased", ch: make(chan struct{}, 1)}
		leaser := &fakeLeaser{grants: []bool{grant}}
		s := NewScheduler(logger.NewNopLogger()).WithLeases(leaser, "replica-a")
		s.Register(job, time.Hour)
		runStartupOnly(t, s)

		if leaser.count() < 1 {
			t.Fatalf("grant=%v: expected a lease attempt", grant)
		}
		want := int64(0)
		if grant {
			want = 1
		}
		if got := atomic.LoadInt64(&job.runs); got != want {
			t.Fatalf("grant=%v: expected %d runs, got %d", grant, want, got)
		}
	}
}

func TestScheduler_SkipsWhenLeaseStoreFails(t *testing.T) {
	job := &signalJob{name: "leased", ch: make(chan struct{}, 1)}
	leaser := &fakeLeaser{err: errors.New("mongo down")}
	s := NewScheduler(logger.NewNopLogger()).WithLeases(leaser, "replica-a")
	s.Register(job, time.Hour)
	runStartupOnly(t, s)

	if got := atomic.LoadInt64(&job.runs); got != 0 {
		t.Fatalf("job must not run without a lease, ran %d times", got)
	}
}

// blockingJob runs until its context is cancelled and records why it stopped.
type blockingJob struct {
	stopped chan error
}

func (j *blockingJob) Name() string { return "blocking" }

func (j *blockingJob) Run(ctx context.Context) error {
	<-ctx.Done()
	j.stopped <- ctx.Err()

	return nil
}

// A renewal refused mid-run (another replica took the lease, or the store is
// gone) cancels the running job's context.
func TestScheduler_CancelsJobWhenLeaseIsLost(t *testing.T) {
	job := &blockingJob{stopped: make(chan error, 1)}
	leaser := &fakeLeaser{grants: []bool{true, false}} // acquired, then every renewal refused
	s := NewScheduler(logger.NewNopLogger()).WithLeases(leaser, "replica-a")
	s.Register(job, 60*time.Millisecond) // heartbeat every 20ms

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	select {
	case err := <-job.stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job was not cancelled after losing its lease")
	}
	if leaser.count() < 2 {
		t.Fatalf("expected an acquire and at least one renewal, got %d calls", leaser.count())
	}
}
