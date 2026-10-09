//go:build integration

package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	"github.com/aspirit/xsocial/internal/storage/mongo/indexes"
	"github.com/aspirit/xsocial/internal/storage/mongo/repository"
	"github.com/aspirit/xsocial/internal/testutil/mongotest"
)

var errRollback = errors.New("rollback on purpose")

// TestPostLike_ConcurrentLikesAreIdempotent hammers the same (post, user) pair
// from many goroutines: the deterministic _id + $setOnInsert upsert must create
// exactly one relation, report "created" exactly once, and never surface the
// duplicate-key race to callers.
func TestPostLike_ConcurrentLikesAreIdempotent(t *testing.T) {
	db := mongotest.NewDB(t)
	likes := repository.NewPostLikeRepository(db)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	created := 0
	var errs []error
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := likes.Like(ctx, model.NewPostLike(7, 99))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			}
			if ok {
				created++
			}
		}()
	}
	wg.Wait()

	assert.Empty(t, errs, "every concurrent like must succeed")
	assert.Equal(t, 1, created)
	counts, err := likes.CountByPosts(ctx, []uint64{7})
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts[7])
}

// TestWithTransaction_RollsBack proves the like + counter pair really is
// atomic: when the callback fails after the like is written, the like is gone.
// Needs a replica set (CI); on the standalone dev stack the test is skipped.
func TestWithTransaction_RollsBack(t *testing.T) {
	db := mongotest.NewDB(t)
	mongotest.RequireTransactions(t, db)
	posts := repository.NewPostRepository(db)
	likes := repository.NewPostLikeRepository(db)
	ctx := context.Background()

	require.NoError(t, posts.Create(ctx, model.NewPost(1, 42, "hello")))
	err := posts.WithTransaction(ctx, func(txCtx context.Context) error {
		created, err := likes.Like(txCtx, model.NewPostLike(1, 99))
		require.NoError(t, err)
		require.True(t, created)
		require.NoError(t, posts.IncLikeCount(txCtx, 1, 1))

		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	counts, err := likes.CountByPosts(ctx, []uint64{1})
	require.NoError(t, err)
	assert.Equal(t, int64(0), counts[1], "the like must have been rolled back")
	stored, err := posts.LikeCounters(ctx, []uint64{1})
	require.NoError(t, err)
	assert.Equal(t, model.LikeCounter{}, stored[1], "the counter and its version must have been rolled back")
}

func TestPost_KeysetPaginationAndCounters(t *testing.T) {
	db := mongotest.NewDB(t)
	posts := repository.NewPostRepository(db)
	ctx := context.Background()

	for id := uint64(1); id <= 5; id++ {
		require.NoError(t, posts.Create(ctx, model.NewPost(id, 1, "p")))
	}
	page, err := posts.ListLatest(ctx, 0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, uint64(5), page[0].ID)
	next, err := posts.ListLatest(ctx, page[1].ID, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint64{3, 2}, []uint64{next[0].ID, next[1].ID})

	require.NoError(t, posts.IncLikeCount(ctx, 1, 3))
	stored, err := posts.LikeCounters(ctx, []uint64{1})
	require.NoError(t, err)
	assert.Equal(t, model.LikeCounter{Count: 3, Version: 1}, stored[1])

	ok, err := posts.CorrectLikeCount(ctx, 1, 0, 10)
	require.NoError(t, err)
	assert.False(t, ok, "a stale version must not overwrite the counter")
	ok, err = posts.CorrectLikeCount(ctx, 1, 1, 10)
	require.NoError(t, err)
	assert.True(t, ok)
	stored, err = posts.LikeCounters(ctx, []uint64{1})
	require.NoError(t, err)
	assert.Equal(t, model.LikeCounter{Count: 10, Version: 2}, stored[1], "a correction bumps the version too")
}

// TestPost_CorrectionRejectsABA replays the sequence a value-based check gets
// wrong: read (10) → like (11) → aggregate sees 11 → unlike (10) → correct to 11.
// The counter is back at its original VALUE, but its VERSION moved on twice.
func TestPost_CorrectionRejectsABA(t *testing.T) {
	db := mongotest.NewDB(t)
	posts := repository.NewPostRepository(db)
	ctx := context.Background()
	require.NoError(t, posts.Create(ctx, model.NewPost(1, 42, "p")))
	require.NoError(t, posts.IncLikeCount(ctx, 1, 10))

	read, err := posts.LikeCounters(ctx, []uint64{1}) // reconciler reads (10, v1)
	require.NoError(t, err)
	require.NoError(t, posts.IncLikeCount(ctx, 1, 1))  // like: 11
	require.NoError(t, posts.IncLikeCount(ctx, 1, -1)) // unlike: 10 again

	ok, err := posts.CorrectLikeCount(ctx, 1, read[1].Version, 11) // stale aggregate said 11
	require.NoError(t, err)
	assert.False(t, ok, "the correction must be refused")
	now, err := posts.LikeCounters(ctx, []uint64{1})
	require.NoError(t, err)
	assert.Equal(t, int64(10), now[1].Count)
}

func likeEvent(uid, actor, postID, eventID uint64) model.NotificationEvent {
	return model.NotificationEvent{
		UID: uid, ActorID: actor, ActType: int8(model.UserActTypeLike),
		PostType: model.PostTypePost, PostID: postID, EventID: eventID,
	}
}

// TestNotification_MergeIsIdempotentPerEvent: the same event delivered twice
// counts once; a different event for the same target counts again.
func TestNotification_MergeIsIdempotentPerEvent(t *testing.T) {
	db := mongotest.NewDB(t)
	repo := repository.NewNotificationRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, model.NewNotification(1, likeEvent(42, 10, 500, 9001))))

	res, err := repo.MergeEvent(ctx, likeEvent(42, 10, 500, 9001), 10, 1000)
	require.NoError(t, err)
	assert.Equal(t, interfaces.MergeAlreadyApplied, res, "redelivery of event 9001 must not change the entry")

	res, err = repo.MergeEvent(ctx, likeEvent(42, 11, 500, 9002), 10, 1000)
	require.NoError(t, err)
	assert.Equal(t, interfaces.MergeApplied, res)

	res, err = repo.MergeEvent(ctx, likeEvent(42, 11, 501, 9003), 10, 1000)
	require.NoError(t, err)
	assert.Equal(t, interfaces.MergeNoUnreadEntry, res, "another post is another target")

	inbox, err := repo.FindByUIDWithOffset(ctx, 42, 10, 0)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	assert.Equal(t, int64(2), inbox[0].WhoCount)
	assert.ElementsMatch(t, []uint64{10, 11}, inbox[0].Who)
	assert.ElementsMatch(t, []uint64{9001, 9002}, inbox[0].EventIDs)
}

// TestNotification_UniqueEventIndexSpansReadEntries pins the guarantee the
// consumer relies on, with the exact interleavings a lookup-based check gets
// wrong: a delivery that has already decided "no unread entry" (or "merge into
// this unread entry") when the recipient reads the inbox.
func TestNotification_UniqueEventIndexSpansReadEntries(t *testing.T) {
	db := mongotest.NewDB(t)
	repo := repository.NewNotificationRepository(db)
	ctx := context.Background()
	e := likeEvent(42, 10, 500, 777)

	// Delivery B observed "no unread entry" here (its MergeEvent ran before A's write).
	res, err := repo.MergeEvent(ctx, e, 10, 1000)
	require.NoError(t, err)
	require.Equal(t, interfaces.MergeNoUnreadEntry, res)

	// Delivery A applies E, and the recipient reads the entry.
	require.NoError(t, repo.Create(ctx, model.NewNotification(1, e)))
	ok, err := repo.MarkAsReadForUser(ctx, 42, 1)
	require.NoError(t, err)
	require.True(t, ok)

	// B resumes with a stale decision and tries to create: the event index says no.
	err = repo.Create(ctx, model.NewNotification(2, e))
	require.ErrorIs(t, err, internalerr.ErrDuplicateEvent)

	// A fresh event opens a new unread entry; a stale merge of E into it is refused too.
	require.NoError(t, repo.Create(ctx, model.NewNotification(3, likeEvent(42, 11, 500, 778))))
	res, err = repo.MergeEvent(ctx, e, 10, 1000)
	require.NoError(t, err)
	assert.Equal(t, interfaces.MergeAlreadyApplied, res)

	inbox, err := repo.FindByUIDWithOffset(ctx, 42, 10, 0)
	require.NoError(t, err)
	require.Len(t, inbox, 2)
	for _, n := range inbox {
		assert.Len(t, n.EventIDs, 1, "every event is recorded exactly once")
	}
	unread, err := repo.CountUnreadByUID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, int64(1), unread)

	// And the other unique index still reports the other kind of duplicate.
	err = repo.Create(ctx, model.NewNotification(4, likeEvent(42, 12, 500, 779)))
	require.ErrorIs(t, err, internalerr.ErrDuplicateRecord, "second unread entry for the target")
}

// TestNotification_ConcurrentEventsConvergeOnOneEntry fires many distinct events
// for one target at once, starting from an empty inbox: the create race is
// resolved by the unique partial index and every event is counted exactly once.
func TestNotification_ConcurrentEventsConvergeOnOneEntry(t *testing.T) {
	db := mongotest.NewDB(t)
	repo := repository.NewNotificationRepository(db)
	ctx := context.Background()

	const n = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ev := likeEvent(42, uint64(100+i%5), 500, uint64(7000+i)) // 5 distinct actors
			if err := createOrMerge(ctx, repo, ev, uint64(1+i)); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, errs)

	inbox, err := repo.FindByUIDWithOffset(ctx, 42, 10, 0)
	require.NoError(t, err)
	require.Len(t, inbox, 1, "exactly one unread entry for the target")
	assert.Equal(t, int64(n), inbox[0].WhoCount)
	assert.Len(t, inbox[0].Who, 5)
	assert.Len(t, inbox[0].EventIDs, n)
}

// createOrMerge mirrors the domain's loop (internal/domains/notification) at the
// repository level so the test does not depend on the domain package.
func createOrMerge(ctx context.Context, repo *repository.NotificationRepository, ev model.NotificationEvent, id uint64) error {
	for range 2 {
		res, err := repo.MergeEvent(ctx, ev, 10, 1000)
		if err != nil {
			return err
		}
		if res != interfaces.MergeNoUnreadEntry {
			return nil
		}
		err = repo.Create(ctx, model.NewNotification(id, ev))
		if err == nil || errors.Is(err, internalerr.ErrDuplicateEvent) {
			return nil
		}
		if !errors.Is(err, internalerr.ErrDuplicateRecord) {
			return err
		}
	}

	return errors.New("lost the create race twice")
}

// TestNotification_RedeliveryAfterReadIsRemembered: once the recipient has read
// the entry, a redelivery of an event it absorbed must not create a new one.
func TestNotification_RedeliveryAfterReadIsRemembered(t *testing.T) {
	db := mongotest.NewDB(t)
	repo := repository.NewNotificationRepository(db)
	ctx := context.Background()

	ev := likeEvent(42, 10, 500, 777)
	require.NoError(t, createOrMerge(ctx, repo, ev, 1))
	ok, err := repo.MarkAsReadForUser(ctx, 42, 1)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, createOrMerge(ctx, repo, ev, 2)) // redelivery

	total, err := repo.CountByUID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	unread, err := repo.CountUnreadByUID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, int64(0), unread)
}

func TestNotification_ReadEntryStartsANewOne(t *testing.T) {
	db := mongotest.NewDB(t)
	repo := repository.NewNotificationRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, model.NewNotification(1, likeEvent(42, 10, 500, 1))))
	ok, err := repo.MarkAsReadForUser(ctx, 42, 1)
	require.NoError(t, err)
	require.True(t, ok)

	res, err := repo.MergeEvent(ctx, likeEvent(42, 11, 500, 2), 10, 1000)
	require.NoError(t, err)
	assert.Equal(t, interfaces.MergeNoUnreadEntry, res, "a read entry is never reopened")
	require.NoError(t, repo.Create(ctx, model.NewNotification(2, likeEvent(42, 11, 500, 2))))

	unread, err := repo.CountUnreadByUID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, int64(1), unread)
	total, err := repo.CountByUID(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
}

func TestJobLease_OneHolderPerInterval(t *testing.T) {
	db := mongotest.NewDB(t)
	leases := repository.NewJobLeaseRepository(db)
	ctx := context.Background()

	ok, err := leases.TryAcquire(ctx, "reconcile", "worker-a", 200*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ok, "a free lease is granted")

	ok, err = leases.TryAcquire(ctx, "reconcile", "worker-b", 200*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, ok, "a live lease is refused to another holder")

	ok, err = leases.TryAcquire(ctx, "reconcile", "worker-a", 200*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ok, "the holder may renew")

	time.Sleep(250 * time.Millisecond)
	ok, err = leases.TryAcquire(ctx, "reconcile", "worker-b", 200*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ok, "an expired lease is granted to the next replica")
}

// Startup runs EnsureIndexes on every boot; it must be a no-op the second time,
// unique and partial indexes included.
func TestEnsureIndexes_IsIdempotent(t *testing.T) {
	db := mongotest.NewDB(t) // already ensured once
	require.NoError(t, indexes.EnsureIndexes(context.Background(), db))
}
