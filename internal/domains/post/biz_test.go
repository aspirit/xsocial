package post_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aspirit/xsocial/internal/domains/notification"
	"github.com/aspirit/xsocial/internal/domains/post"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/testutil/fake"
)

const (
	author = uint64(1)
	reader = uint64(2)
)

type fixture struct {
	biz   post.IPostBiz
	posts *fake.Posts
	likes *fake.Likes
	pub   *fake.Publisher
	cache *fake.Cache
}

func newFixture() *fixture {
	f := &fixture{posts: fake.NewPosts(), likes: fake.NewLikes(), pub: &fake.Publisher{}, cache: fake.NewCache()}
	ids := &fake.IDGen{}
	events := notification.NewNotificationEventBiz(f.pub, ids)
	f.biz = post.NewPostBiz(f.posts, f.likes, events, f.cache, ids)

	return f
}

func (f *fixture) create(t *testing.T, uid uint64, content string) uint64 {
	t.Helper()
	p, err := f.biz.Create(context.Background(), uid, &post.CreatePostRequest{Content: content})
	require.NoError(t, err)
	var id uint64
	require.NoError(t, json.Unmarshal([]byte(p.ID), &id))

	return id
}

func TestCreate_Validation(t *testing.T) {
	f := newFixture()
	ctx := context.Background()

	_, err := f.biz.Create(ctx, author, &post.CreatePostRequest{Content: "   "})
	assert.ErrorIs(t, err, post.ErrContentEmpty)

	_, err = f.biz.Create(ctx, author, &post.CreatePostRequest{Content: strings.Repeat("字", post.MaxContentRunes+1)})
	assert.ErrorIs(t, err, post.ErrContentTooBig)

	// Limit counts runes, not bytes: exactly MaxContentRunes CJK characters is fine.
	_, err = f.biz.Create(ctx, author, &post.CreatePostRequest{Content: strings.Repeat("字", post.MaxContentRunes)})
	assert.NoError(t, err)
}

func TestLike_IsIdempotentAndNotifiesOnce(t *testing.T) {
	f := newFixture()
	id := f.create(t, author, "hello")
	ctx := context.Background()

	for range 3 { // double clicks / client retries
		got, err := f.biz.Like(ctx, id, reader)
		require.NoError(t, err)
		assert.Equal(t, int64(1), got.LikeCount)
		assert.True(t, got.LikedByMe)
	}
	assert.Equal(t, 1, f.pub.Count(), "exactly one notification event for one logical like")

	var payload notification.NotificationPayload
	require.NoError(t, json.Unmarshal(f.pub.Msgs[0].Value, &payload))
	assert.Equal(t, author, payload.UID)
	assert.Equal(t, reader, payload.ActorID)
	assert.Equal(t, int8(model.UserActTypeLike), payload.ActType)
	assert.Equal(t, notification.NotificationEventVersion, payload.Version)
}

func TestLike_SelfLikeDoesNotNotify(t *testing.T) {
	f := newFixture()
	id := f.create(t, author, "hello")

	_, err := f.biz.Like(context.Background(), id, author)
	require.NoError(t, err)
	assert.Equal(t, 0, f.pub.Count())
}

func TestLike_BrokerOutageDoesNotFailTheRequest(t *testing.T) {
	f := newFixture()
	id := f.create(t, author, "hello")
	f.pub.Err = errors.New("broker down")

	got, err := f.biz.Like(context.Background(), id, reader)
	require.NoError(t, err, "the like is committed; notification is best-effort")
	assert.Equal(t, int64(1), got.LikeCount)
}

func TestUnlike_RestoresCounterAndIsIdempotent(t *testing.T) {
	f := newFixture()
	id := f.create(t, author, "hello")
	ctx := context.Background()

	_, err := f.biz.Like(ctx, id, reader)
	require.NoError(t, err)
	for range 2 {
		got, err := f.biz.Unlike(ctx, id, reader)
		require.NoError(t, err)
		assert.Equal(t, int64(0), got.LikeCount)
		assert.False(t, got.LikedByMe)
	}
}

func TestGet_UsesCacheAndInvalidatesOnWrite(t *testing.T) {
	f := newFixture()
	id := f.create(t, author, "hello")
	ctx := context.Background()

	_, err := f.biz.Get(ctx, id, 0)
	require.NoError(t, err)
	_, err = f.biz.Get(ctx, id, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, f.posts.FindCalls, "second read is served from cache")

	_, err = f.biz.Like(ctx, id, reader) // write invalidates the cached post
	require.NoError(t, err)
	got, err := f.biz.Get(ctx, id, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got.LikeCount, "no stale counter after invalidation")
}

func TestGet_NotFound(t *testing.T) {
	f := newFixture()
	_, err := f.biz.Get(context.Background(), 424242, 0)
	assert.ErrorIs(t, err, post.ErrPostNotFound)

	_, err = f.biz.Like(context.Background(), 424242, reader)
	assert.ErrorIs(t, err, post.ErrPostNotFound)
}

func TestListLatest_KeysetPagination(t *testing.T) {
	f := newFixture()
	for i := range 5 {
		f.create(t, author, "post "+string(rune('a'+i)))
	}
	ctx := context.Background()

	page1, err := f.biz.ListLatest(ctx, 0, &post.ListRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.Posts, 2)
	require.NotEmpty(t, page1.NextCursor)

	page2, err := f.biz.ListLatest(ctx, 0, &post.ListRequest{Limit: 2, Cursor: page1.NextCursor})
	require.NoError(t, err)
	require.Len(t, page2.Posts, 2)
	assert.Greater(t, page1.Posts[1].ID, page2.Posts[0].ID, "pages are newest-first and do not overlap")

	page3, err := f.biz.ListLatest(ctx, 0, &post.ListRequest{Limit: 2, Cursor: page2.NextCursor})
	require.NoError(t, err)
	assert.Len(t, page3.Posts, 1)
	assert.Empty(t, page3.NextCursor, "last page has no cursor")

	_, err = f.biz.ListLatest(ctx, 0, &post.ListRequest{Limit: 2, Cursor: "not-a-number"})
	assert.ErrorIs(t, err, post.ErrInvalidParams)
}

// staleRefill reproduces the cache-aside race deterministically: a reader that
// loaded the post before a like committed writes its old copy back right after
// the like's invalidation.
type staleRefill struct {
	*fake.Cache
	key, stale string
}

func (c *staleRefill) Del(ctx context.Context, keys ...string) error {
	if err := c.Cache.Del(ctx, keys...); err != nil {
		return err
	}

	return c.Set(ctx, c.key, c.stale, time.Minute)
}

// The cache may serve the old counter for up to its TTL, but the actor's own
// like response is read from the database and is always current.
func TestLike_ResponseIsFreshEvenWhenCacheIsRefilledWithStaleData(t *testing.T) {
	ctx := context.Background()
	posts, likes, ids := fake.NewPosts(), fake.NewLikes(), &fake.IDGen{}
	base := fake.NewCache()
	biz := post.NewPostBiz(posts, likes, nil, base, ids)

	created, err := biz.Create(ctx, author, &post.CreatePostRequest{Content: "hello"})
	require.NoError(t, err)
	var id uint64
	require.NoError(t, json.Unmarshal([]byte(created.ID), &id))
	_, err = biz.Get(ctx, id, 0) // populate the cache with likeCount 0
	require.NoError(t, err)
	stale, err := base.Get(ctx, post.CacheKey(id))
	require.NoError(t, err)

	racy := &staleRefill{Cache: base, key: post.CacheKey(id), stale: stale}
	biz = post.NewPostBiz(posts, likes, nil, racy, ids)

	liked, err := biz.Like(ctx, id, reader)
	require.NoError(t, err)
	assert.Equal(t, int64(1), liked.LikeCount, "the actor sees the committed count")
	assert.True(t, liked.LikedByMe)

	cached, err := biz.Get(ctx, id, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), cached.LikeCount, "documented trade-off: other readers may see the stale copy until the TTL expires")
}
