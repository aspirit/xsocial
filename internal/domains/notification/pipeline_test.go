package notification_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aspirit/xsocial/internal/domains/notification"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/testutil/fake"
)

type pipeline struct {
	pub      *fake.Publisher
	store    *fake.Notifications
	biz      notification.INotificationBiz
	producer notification.INotificationEventBiz
	consumer *notification.NotificationHandler
}

// newPipeline wires the full asynchronous path in-process:
// producer (EnqueueNotificationEvent) → broker (fake) → consumer (Handle) → store.
func newPipeline() *pipeline {
	pub := &fake.Publisher{}
	ids := &fake.IDGen{}
	store := fake.NewNotifications()
	biz := notification.NewNotificationBiz(store, ids, nil)

	return &pipeline{
		pub:      pub,
		store:    store,
		biz:      biz,
		producer: notification.NewNotificationEventBiz(pub, ids),
		consumer: notification.NewNotificationHandler(biz),
	}
}

func (p *pipeline) like(t *testing.T, author, actor, postID uint64) {
	t.Helper()
	require.NoError(t, p.producer.EnqueueNotificationEvent(context.Background(), author, actor,
		model.UserActTypeLike, model.NotificationEventEntityPost, postID, nil))
}

func (p *pipeline) drain(t *testing.T) {
	t.Helper()
	for _, msg := range p.pub.Msgs {
		require.NoError(t, p.consumer.Handle(context.Background(), msg))
	}
	p.pub.Msgs = nil
}

func TestEventPipeline_MergesActorsPerTarget(t *testing.T) {
	p := newPipeline()
	const author, postID = uint64(1), uint64(500)
	for _, actor := range []uint64{10, 11, 12, 11} { // 11 likes twice (re-like after unlike)
		p.like(t, author, actor, postID)
	}
	p.drain(t)

	all := p.store.All()
	require.Len(t, all, 1, "events for the same (recipient, action, target) merge into one entry")
	assert.ElementsMatch(t, []uint64{10, 11, 12}, all[0].Who, "who holds distinct actors")
	assert.Equal(t, int64(4), all[0].WhoCount, "who_count counts every event")
}

func TestEventPipeline_RedeliveryIsIdempotent(t *testing.T) {
	p := newPipeline()
	p.like(t, 1, 10, 500)
	msg := p.pub.Msgs[0]

	// At-least-once delivery: the broker hands the same message over three times,
	// including after the entry already exists.
	for range 3 {
		require.NoError(t, p.consumer.Handle(context.Background(), msg))
	}

	all := p.store.All()
	require.Len(t, all, 1)
	assert.Equal(t, int64(1), all[0].WhoCount, "one event id counts once, however often it is delivered")
	assert.Equal(t, []uint64{10}, all[0].Who)
}

func TestEventPipeline_ReadEntryIsNotReopened(t *testing.T) {
	p := newPipeline()
	p.like(t, 1, 10, 500)
	p.drain(t)
	require.NoError(t, p.biz.MarkAllAsRead(context.Background(), 1))

	// A new event after the recipient has read the entry starts a fresh one.
	p.like(t, 1, 11, 500)
	p.drain(t)

	all := p.store.All()
	require.Len(t, all, 2)
	unread, err := p.biz.GetUnreadCount(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), unread)
}

// A redelivery that arrives after the recipient has read the entry must not
// reopen the inbox: the read entry still remembers the event id.
func TestEventPipeline_RedeliveryAfterReadIsIgnored(t *testing.T) {
	p := newPipeline()
	p.like(t, 1, 10, 500)
	msg := p.pub.Msgs[0]
	p.drain(t)
	require.NoError(t, p.biz.MarkAllAsRead(context.Background(), 1))

	require.NoError(t, p.consumer.Handle(context.Background(), msg))

	assert.Len(t, p.store.All(), 1, "no new entry for an event that was already applied")
	unread, err := p.biz.GetUnreadCount(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), unread)
}

// The same, while a NEW unread entry already exists: the stale redelivery must
// not be merged into it either.
func TestEventPipeline_RedeliveryAfterReadDoesNotLeakIntoNewEntry(t *testing.T) {
	p := newPipeline()
	p.like(t, 1, 10, 500)
	stale := p.pub.Msgs[0]
	p.drain(t)
	require.NoError(t, p.biz.MarkAllAsRead(context.Background(), 1))
	p.like(t, 1, 11, 500) // fresh event opens a new unread entry
	p.drain(t)

	require.NoError(t, p.consumer.Handle(context.Background(), stale))

	inbox, err := p.biz.GetNotificationsByUserID(context.Background(), 1, 10, 0)
	require.NoError(t, err)
	require.Len(t, inbox.Notifications, 2)
	assert.Equal(t, int64(1), inbox.UnreadCount)
	assert.Equal(t, int64(1), inbox.Notifications[0].WhoCount, "the new entry counts only its own event")
}

func TestEventPipeline_SeparateTargetsDoNotMerge(t *testing.T) {
	p := newPipeline()
	p.like(t, 1, 10, 500)
	p.like(t, 1, 10, 501) // same actor, other post
	p.like(t, 2, 10, 500) // other recipient
	p.drain(t)

	assert.Len(t, p.store.All(), 3)
}

func TestMarkAsRead_IsScopedToOwner(t *testing.T) {
	ctx := context.Background()
	p := newPipeline()
	require.NoError(t, p.biz.CreateOrMergeNotification(ctx, model.NotificationEvent{
		UID: 1, ActorID: 10, ActType: int8(model.UserActTypeLike), PostType: model.PostTypePost, PostID: 500, EventID: 77,
	}))
	id := p.store.All()[0].ID

	err := p.biz.MarkAsRead(ctx, 2, formatID(id)) // someone else's notification
	assert.Error(t, err)
	unread, _ := p.biz.GetUnreadCount(ctx, 1)
	assert.Equal(t, int64(1), unread)

	require.NoError(t, p.biz.MarkAsRead(ctx, 1, formatID(id)))
	unread, _ = p.biz.GetUnreadCount(ctx, 1)
	assert.Equal(t, int64(0), unread)
}
