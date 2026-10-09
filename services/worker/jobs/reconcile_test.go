package jobs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/pkg/logger"
)

// fakeLikeSource is an in-memory LikeCountSource. onExpected runs while the
// reconciler is between its two reads, to simulate concurrent traffic; it may
// return the aggregate the reconciler should observe at that moment.
type fakeLikeSource struct {
	ids        []uint64
	expected   map[uint64]int64
	stored     map[uint64]model.LikeCounter
	sets       int
	onExpected func() map[uint64]int64
}

func (f *fakeLikeSource) NextPostIDs(_ context.Context, after uint64, limit int64) ([]uint64, error) {
	var out []uint64
	for _, id := range f.ids {
		if id > after && int64(len(out)) < limit {
			out = append(out, id)
		}
	}

	return out, nil
}

func (f *fakeLikeSource) ExpectedLikeCounts(_ context.Context, ids []uint64) (map[uint64]int64, error) {
	if f.onExpected != nil {
		if seen := f.onExpected(); seen != nil {
			return pick(seen, ids), nil
		}
	}

	return pick(f.expected, ids), nil
}

func (f *fakeLikeSource) StoredLikeCounters(_ context.Context, ids []uint64) (map[uint64]model.LikeCounter, error) {
	out := map[uint64]model.LikeCounter{}
	for _, id := range ids {
		if v, ok := f.stored[id]; ok {
			out[id] = v
		}
	}

	return out, nil
}

func (f *fakeLikeSource) CorrectLikeCount(_ context.Context, id uint64, version, want int64) (bool, error) {
	if f.stored[id].Version != version {
		return false, nil
	}
	f.stored[id] = model.LikeCounter{Count: want, Version: version + 1}
	f.sets++

	return true, nil
}

// like simulates a like (+1) or unlike (-1) committed by live traffic.
func (f *fakeLikeSource) like(id uint64, delta int64) {
	f.expected[id] += delta
	c := f.stored[id]
	f.stored[id] = model.LikeCounter{Count: c.Count + delta, Version: c.Version + 1}
}

func counters(m map[uint64]int64) map[uint64]model.LikeCounter {
	out := map[uint64]model.LikeCounter{}
	for id, n := range m {
		out[id] = model.LikeCounter{Count: n}
	}

	return out
}

func pick(m map[uint64]int64, ids []uint64) map[uint64]int64 {
	out := map[uint64]int64{}
	for _, id := range ids {
		if v, ok := m[id]; ok {
			out[id] = v
		}
	}

	return out
}

type countingRecorder struct{ total int64 }

func (c *countingRecorder) RecordDrift(_ context.Context, d int64) { c.total += d }

func newTestLogger() *logger.Logger { return logger.NewNopLogger() }

func TestLikeCountReconciler_CorrectsDriftAcrossPages(t *testing.T) {
	src := &fakeLikeSource{
		ids:      []uint64{1, 2, 3, 4, 5},
		expected: map[uint64]int64{1: 3, 2: 0, 3: 7, 5: 1},
		stored:   counters(map[uint64]int64{1: 3, 2: 2, 3: 5, 4: 0, 5: 1}),
	}
	rec := &countingRecorder{}
	r := NewLikeCountReconciler(src, rec, newTestLogger(), true)
	r.pageSize = 2 // force several pages

	require.NoError(t, r.Run(context.Background()))

	assert.Equal(t, int64(0), src.stored[2].Count)
	assert.Equal(t, int64(7), src.stored[3].Count)
	assert.Equal(t, 2, src.sets)
	assert.Equal(t, int64(4), rec.total) // |0-2| + |7-5|
}

func TestLikeCountReconciler_IsIdempotent(t *testing.T) {
	src := &fakeLikeSource{
		ids:      []uint64{1},
		expected: map[uint64]int64{1: 4},
		stored:   counters(map[uint64]int64{1: 1}),
	}
	r := NewLikeCountReconciler(src, nil, newTestLogger(), true)
	require.NoError(t, r.Run(context.Background()))
	require.NoError(t, r.Run(context.Background()))
	assert.Equal(t, 1, src.sets, "second run must find no drift")
}

func TestLikeCountReconciler_DryRunDoesNotWrite(t *testing.T) {
	src := &fakeLikeSource{
		ids:      []uint64{1},
		expected: map[uint64]int64{1: 4},
		stored:   counters(map[uint64]int64{1: 1}),
	}
	r := NewLikeCountReconciler(src, nil, newTestLogger(), false)
	require.NoError(t, r.Run(context.Background()))
	assert.Equal(t, 0, src.sets)
	assert.Equal(t, int64(1), src.stored[1].Count)
}

// A like that commits while the reconciler is between its two reads must never
// be undone: the correction is a compare-and-set on the version it read.
func TestLikeCountReconciler_DoesNotClobberConcurrentLike(t *testing.T) {
	src := &fakeLikeSource{
		ids:      []uint64{1},
		expected: map[uint64]int64{1: 10},
		stored:   counters(map[uint64]int64{1: 10}),
	}
	src.onExpected = func() map[uint64]int64 {
		src.like(1, +1) // commits after the counter was read: relation and counter both 11

		return nil
	}
	r := NewLikeCountReconciler(src, nil, newTestLogger(), true)
	require.NoError(t, r.Run(context.Background()))

	assert.Equal(t, int64(11), src.stored[1].Count, "the fresh counter must survive")
	assert.Equal(t, 0, src.sets, "the compare-and-set must have been refused")
}

// ABA: read 10 → like (11) → aggregate observed 11 → unlike (10). The value is
// back to what was read, but writing 11 would be wrong; the version says no.
func TestLikeCountReconciler_RejectsABA(t *testing.T) {
	src := &fakeLikeSource{
		ids:      []uint64{1},
		expected: map[uint64]int64{1: 10},
		stored:   counters(map[uint64]int64{1: 10}),
	}
	src.onExpected = func() map[uint64]int64 {
		src.like(1, +1)
		observed := map[uint64]int64{1: src.expected[1]} // the aggregate ran here: 11
		src.like(1, -1)

		return observed
	}
	r := NewLikeCountReconciler(src, nil, newTestLogger(), true)
	require.NoError(t, r.Run(context.Background()))

	assert.Equal(t, int64(10), src.stored[1].Count, "10 is the truth; a value-based CAS would have written 11")
	assert.Equal(t, 0, src.sets)
}
