package post

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/domains/notification"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/idgen"
	"github.com/aspirit/xsocial/pkg/logger"
)

// postCacheTTL bounds staleness of cached posts; writes invalidate explicitly.
const postCacheTTL = 5 * time.Minute

// IPostBiz is the post domain contract.
type IPostBiz interface {
	Create(ctx context.Context, uid uint64, req *CreatePostRequest) (*PostObject, error)
	Get(ctx context.Context, postID, viewerUID uint64) (*PostObject, error)
	ListLatest(ctx context.Context, viewerUID uint64, req *ListRequest) (*PostCollection, error)
	ListByUser(ctx context.Context, authorUID, viewerUID uint64, req *ListRequest) (*PostCollection, error)
	Like(ctx context.Context, postID, uid uint64) (*PostObject, error)
	Unlike(ctx context.Context, postID, uid uint64) (*PostObject, error)
}

// PostBiz implements IPostBiz.
//
// Write path for a like:
//
//  1. MongoDB transaction: idempotent upsert of the like relation + $inc like_count
//  2. Redis: invalidate the cached post (cache-aside)
//  3. Broker: publish a notification event, synchronously and best-effort, after
//     the commit; the worker merges it into the author's inbox later. The publish
//     round-trip is part of the request, the inbox work is not.
type PostBiz struct {
	posts  interfaces.IPostRepository
	likes  interfaces.IPostLikeRepository
	events notification.INotificationEventBiz
	cache  cache.Cache // optional; nil disables caching
	idGen  idgen.Generator
}

// NewPostBiz creates the post domain service.
func NewPostBiz(
	posts interfaces.IPostRepository,
	likes interfaces.IPostLikeRepository,
	events notification.INotificationEventBiz,
	c cache.Cache,
	idGen idgen.Generator,
) IPostBiz {
	return &PostBiz{posts: posts, likes: likes, events: events, cache: c, idGen: idGen}
}

// Create validates and stores a post.
func (b *PostBiz) Create(ctx context.Context, uid uint64, req *CreatePostRequest) (*PostObject, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, ErrContentEmpty
	}
	if utf8.RuneCountInString(content) > MaxContentRunes {
		return nil, ErrContentTooBig
	}
	id, err := b.idGen.NextID()
	if err != nil {
		return nil, fmt.Errorf("generate post id: %w", err)
	}
	p := model.NewPost(id, uid, content)
	if err := b.posts.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("insert post: %w", err)
	}
	obj := toObject(p, false)

	return &obj, nil
}

// Get returns one post, served from Redis when warm (cache-aside, fail-open).
func (b *PostBiz) Get(ctx context.Context, postID, viewerUID uint64) (*PostObject, error) {
	p, err := b.load(ctx, postID)
	if err != nil {
		return nil, err
	}

	return b.view(ctx, p, viewerUID)
}

// fresh returns the post as committed in MongoDB, bypassing the cache. Like and
// unlike answer with it: the cache is eventually consistent (a reader that
// loaded the post before the commit can write the old value back after the
// invalidation), but the actor must always see their own write.
func (b *PostBiz) fresh(ctx context.Context, postID, viewerUID uint64) (*PostObject, error) {
	p, err := b.posts.FindByID(ctx, postID)
	if err != nil {
		if errors.Is(err, internalerr.ErrRecordNotFound) {
			return nil, ErrPostNotFound
		}

		return nil, fmt.Errorf("load post %d: %w", postID, err)
	}

	return b.view(ctx, p, viewerUID)
}

// view adds the viewer's "liked by me" flag to a post.
func (b *PostBiz) view(ctx context.Context, p *model.Post, viewerUID uint64) (*PostObject, error) {
	liked := false
	if viewerUID != 0 {
		flags, err := b.likes.LikedBy(ctx, viewerUID, []uint64{p.ID})
		if err != nil {
			return nil, fmt.Errorf("liked flags: %w", err)
		}
		liked = flags[p.ID]
	}
	obj := toObject(p, liked)

	return &obj, nil
}

// ListLatest returns the global feed, newest first.
func (b *PostBiz) ListLatest(ctx context.Context, viewerUID uint64, req *ListRequest) (*PostCollection, error) {
	before, err := parseCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	posts, err := b.posts.ListLatest(ctx, before, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("list latest: %w", err)
	}

	return b.page(ctx, viewerUID, posts, req.Limit)
}

// ListByUser returns one author's posts, newest first.
func (b *PostBiz) ListByUser(ctx context.Context, authorUID, viewerUID uint64, req *ListRequest) (*PostCollection, error) {
	before, err := parseCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	posts, err := b.posts.ListByUID(ctx, authorUID, before, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("list by user: %w", err)
	}

	return b.page(ctx, viewerUID, posts, req.Limit)
}

// Like is idempotent: repeating it changes nothing and emits no extra event.
func (b *PostBiz) Like(ctx context.Context, postID, uid uint64) (*PostObject, error) {
	p, err := b.load(ctx, postID)
	if err != nil {
		return nil, err
	}

	created := false
	err = b.posts.WithTransaction(ctx, func(txCtx context.Context) error {
		var txErr error
		created, txErr = b.likes.Like(txCtx, model.NewPostLike(postID, uid))
		if txErr != nil {
			return fmt.Errorf("insert like: %w", txErr)
		}
		if !created {
			return nil
		}

		return b.posts.IncLikeCount(txCtx, postID, 1)
	})
	if err != nil {
		return nil, fmt.Errorf("like post: %w", err)
	}

	if created {
		cache.Invalidate(ctx, b.cache, CacheKey(postID))
		b.notifyAuthor(ctx, p, uid)
	}

	return b.fresh(ctx, postID, uid)
}

// Unlike is idempotent as well.
func (b *PostBiz) Unlike(ctx context.Context, postID, uid uint64) (*PostObject, error) {
	if _, err := b.load(ctx, postID); err != nil {
		return nil, err
	}

	removed := false
	err := b.posts.WithTransaction(ctx, func(txCtx context.Context) error {
		var txErr error
		removed, txErr = b.likes.Unlike(txCtx, postID, uid)
		if txErr != nil {
			return fmt.Errorf("delete like: %w", txErr)
		}
		if !removed {
			return nil
		}

		return b.posts.IncLikeCount(txCtx, postID, -1)
	})
	if err != nil {
		return nil, fmt.Errorf("unlike post: %w", err)
	}
	if removed {
		cache.Invalidate(ctx, b.cache, CacheKey(postID))
	}

	return b.fresh(ctx, postID, uid)
}

// notifyAuthor publishes the "liked your post" event. It is best-effort: the
// like is already committed, so a broker outage is logged rather than failing
// the request, at the price of a lost event. (A transactional outbox would close
// that gap; see docs/ARCHITECTURE.md.)
func (b *PostBiz) notifyAuthor(ctx context.Context, p *model.Post, actorUID uint64) {
	if b.events == nil || p.UID == actorUID {
		return
	}
	err := b.events.EnqueueNotificationEvent(ctx, p.UID, actorUID,
		model.UserActTypeLike, model.NotificationEventEntityPost, p.ID, nil)
	if err != nil {
		logger.LoggerFromContext(ctx).Warn("enqueue like notification failed",
			zap.Uint64("post_id", p.ID), zap.Uint64("actor", actorUID), zap.Error(err))
	}
}

func (b *PostBiz) load(ctx context.Context, postID uint64) (*model.Post, error) {
	p, err := cache.Aside(ctx, b.cache, CacheKey(postID), postCacheTTL,
		func(ctx context.Context) (*model.Post, error) {
			return b.posts.FindByID(ctx, postID)
		})
	if err != nil {
		if errors.Is(err, internalerr.ErrRecordNotFound) {
			return nil, ErrPostNotFound
		}

		return nil, fmt.Errorf("load post %d: %w", postID, err)
	}

	return p, nil
}

func (b *PostBiz) page(ctx context.Context, viewerUID uint64, posts []model.Post, limit int64) (*PostCollection, error) {
	liked := map[uint64]bool{}
	if viewerUID != 0 && len(posts) > 0 {
		ids := make([]uint64, len(posts))
		for i := range posts {
			ids[i] = posts[i].ID
		}
		var err error
		if liked, err = b.likes.LikedBy(ctx, viewerUID, ids); err != nil {
			return nil, fmt.Errorf("liked flags: %w", err)
		}
	}

	out := &PostCollection{Posts: make([]PostObject, 0, len(posts))}
	for i := range posts {
		out.Posts = append(out.Posts, toObject(&posts[i], liked[posts[i].ID]))
	}
	if int64(len(posts)) == limit && len(posts) > 0 {
		out.NextCursor = strconv.FormatUint(posts[len(posts)-1].ID, 10)
	}

	return out, nil
}

// CacheKey is the cache-aside key of a post; the worker's reconciliation job
// invalidates it after correcting like_count.
func CacheKey(postID uint64) string {
	return cache.Key(cache.NSPost, strconv.FormatUint(postID, 10))
}

func parseCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return 0, ErrInvalidParams
	}

	return v, nil
}
