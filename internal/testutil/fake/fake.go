// Package fake provides in-memory implementations of the storage interfaces,
// the cache and the event publisher. Domain tests run against them, so business
// rules are verified in milliseconds without MongoDB, Redis or Kafka; the real
// adapters are covered separately by integration tests (build tag "integration").
package fake

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/model"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	"github.com/aspirit/xsocial/pkg/cache"
	"github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/idgen"
)

// Compile-time checks: the fakes satisfy the same contracts as production code.
var (
	_ interfaces.IUserInfoRepository     = (*Users)(nil)
	_ interfaces.IPostRepository         = (*Posts)(nil)
	_ interfaces.IPostLikeRepository     = (*Likes)(nil)
	_ interfaces.INotificationRepository = (*Notifications)(nil)
	_ cache.Cache                        = (*Cache)(nil)
	_ eventbroker.Publisher              = (*Publisher)(nil)
	_ idgen.Generator                    = (*IDGen)(nil)
)

// ---------------------------------------------------------------- IDs

// IDGen returns increasing IDs starting at 1000.
type IDGen struct{ n atomic.Uint64 }

func (g *IDGen) NextID() (uint64, error) { return 1000 + g.n.Add(1), nil }
func (g *IDGen) NextShortID() (string, error) {
	id, _ := g.NextID()

	return strconv.FormatUint(id, 10), nil
}
func (g *IDGen) Parse(uint64) (time.Time, uint16, uint16) { return time.Time{}, 0, 0 }
func (g *IDGen) ParseShortID(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse short id: %w", err)
	}

	return v, nil
}

// ---------------------------------------------------------------- users

// Users is an in-memory user store with a unique username constraint.
type Users struct {
	mu   sync.Mutex
	byID map[uint64]model.UserInfo
}

func NewUsers() *Users { return &Users{byID: map[uint64]model.UserInfo{}} }

func (r *Users) FindByID(_ context.Context, id uint64) (*model.UserInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, internalerr.ErrRecordNotFound
	}

	return &u, nil
}

func (r *Users) FindByUsername(_ context.Context, username string) (*model.UserInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.byID {
		if u.Username == username {
			return &u, nil
		}
	}

	return nil, internalerr.ErrRecordNotFound
}

func (r *Users) Create(_ context.Context, u *model.UserInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.byID {
		if existing.Username == u.Username {
			return internalerr.ErrDuplicateRecord
		}
	}
	r.byID[u.ID] = *u

	return nil
}

// ---------------------------------------------------------------- posts

// Posts is an in-memory post store. FindCalls counts primary-store reads so
// tests can assert cache hits.
type Posts struct {
	mu        sync.Mutex
	byID      map[uint64]model.Post
	FindCalls int
}

func NewPosts() *Posts { return &Posts{byID: map[uint64]model.Post{}} }

func (r *Posts) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func (r *Posts) FindByID(_ context.Context, id uint64) (*model.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.FindCalls++
	p, ok := r.byID[id]
	if !ok {
		return nil, internalerr.ErrRecordNotFound
	}

	return &p, nil
}

func (r *Posts) list(filter func(model.Post) bool, beforeID uint64, limit int64) []model.Post {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.Post
	for _, p := range r.byID {
		if filter(p) && (beforeID == 0 || p.ID < beforeID) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if int64(len(out)) > limit {
		out = out[:limit]
	}

	return out
}

func (r *Posts) ListLatest(_ context.Context, beforeID uint64, limit int64) ([]model.Post, error) {
	return r.list(func(model.Post) bool { return true }, beforeID, limit), nil
}

func (r *Posts) ListByUID(_ context.Context, uid, beforeID uint64, limit int64) ([]model.Post, error) {
	return r.list(func(p model.Post) bool { return p.UID == uid }, beforeID, limit), nil
}

func (r *Posts) Create(_ context.Context, p *model.Post) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[p.ID] = *p

	return nil
}

func (r *Posts) IncLikeCount(_ context.Context, id uint64, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.byID[id]
	p.LikeCount += delta
	p.LikeVersion++
	r.byID[id] = p

	return nil
}

func (r *Posts) CorrectLikeCount(_ context.Context, id uint64, version, n int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.byID[id]
	if p.LikeVersion != version {
		return false, nil
	}
	p.LikeCount = n
	p.LikeVersion++
	r.byID[id] = p

	return true, nil
}

func (r *Posts) LikeCounters(_ context.Context, ids []uint64) (map[uint64]model.LikeCounter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[uint64]model.LikeCounter{}
	for _, id := range ids {
		p := r.byID[id]
		out[id] = model.LikeCounter{Count: p.LikeCount, Version: p.LikeVersion}
	}

	return out, nil
}

func (r *Posts) ScanIDs(_ context.Context, afterID uint64, limit int64) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uint64
	for id := range r.byID {
		if id > afterID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if int64(len(ids)) > limit {
		ids = ids[:limit]
	}

	return ids, nil
}

// ---------------------------------------------------------------- likes

// Likes is an in-memory like relation.
type Likes struct {
	mu  sync.Mutex
	set map[string]model.PostLike
}

func NewLikes() *Likes { return &Likes{set: map[string]model.PostLike{}} }

func (r *Likes) Like(_ context.Context, l *model.PostLike) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.set[l.ID]; ok {
		return false, nil
	}
	r.set[l.ID] = *l

	return true, nil
}

func (r *Likes) Unlike(_ context.Context, postID, uid uint64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := model.PostLikeID(postID, uid)
	if _, ok := r.set[id]; !ok {
		return false, nil
	}
	delete(r.set, id)

	return true, nil
}

func (r *Likes) LikedBy(_ context.Context, uid uint64, postIDs []uint64) (map[uint64]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[uint64]bool{}
	for _, id := range postIDs {
		if _, ok := r.set[model.PostLikeID(id, uid)]; ok {
			out[id] = true
		}
	}

	return out, nil
}

func (r *Likes) CountByPosts(_ context.Context, postIDs []uint64) (map[uint64]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[uint64]bool{}
	for _, id := range postIDs {
		want[id] = true
	}
	out := map[uint64]int64{}
	for _, l := range r.set {
		if want[l.PostID] {
			out[l.PostID]++
		}
	}

	return out, nil
}

// ---------------------------------------------------------------- notifications

// Notifications is an in-memory notification store.
type Notifications struct {
	mu   sync.Mutex
	byID map[uint64]model.Notification
}

func NewNotifications() *Notifications {
	return &Notifications{byID: map[uint64]model.Notification{}}
}

// All returns every stored notification (test inspection).
func (r *Notifications) All() []model.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.Notification, 0, len(r.byID))
	for _, n := range r.byID {
		out = append(out, n)
	}

	return out
}

// Create mirrors both unique indexes: an event id on at most one entry per
// uid, and one unread entry per target.
func (r *Notifications) Create(_ context.Context, n *model.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range n.EventIDs {
		if r.holdsEventLocked(n.UID, id) {
			return internalerr.ErrDuplicateEvent
		}
	}
	for _, existing := range r.byID {
		if !existing.IsRead && sameTarget(existing, n.UID, n.ActType, n.PostType, n.PostID) {
			return internalerr.ErrDuplicateRecord
		}
	}
	r.byID[n.ID] = *n

	return nil
}

func sameTarget(n model.Notification, uid uint64, actType int8, postType model.PostType, postID uint64) bool {
	return n.UID == uid && n.ActType == actType && n.PostType == postType && n.PostID == postID
}

// holdsEventLocked reports whether any entry of uid (read or unread) records eventID.
func (r *Notifications) holdsEventLocked(uid, eventID uint64) bool {
	for _, n := range r.byID {
		if n.UID != uid {
			continue
		}
		for _, e := range n.EventIDs {
			if e == eventID {
				return true
			}
		}
	}

	return false
}

func (r *Notifications) FindByUIDWithOffset(_ context.Context, uid uint64, limit, offset int64) ([]model.Notification, error) {
	var out []model.Notification
	for _, n := range r.All() {
		if n.UID == uid {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	if offset >= int64(len(out)) {
		return nil, nil
	}
	out = out[offset:]
	if int64(len(out)) > limit {
		out = out[:limit]
	}

	return out, nil
}

// MergeEvent applies the same rules as the Mongo pipeline update plus the unique
// event index, under one lock.
func (r *Notifications) MergeEvent(_ context.Context, ev model.NotificationEvent, whoMax, eventMax int) (interfaces.MergeResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ev.EventID != 0 && r.holdsEventLocked(ev.UID, ev.EventID) {
		return interfaces.MergeAlreadyApplied, nil // on this entry ($in guard) or another (unique index)
	}
	for id, n := range r.byID {
		if n.IsRead || !sameTarget(n, ev.UID, ev.ActType, ev.PostType, ev.PostID) {
			continue
		}
		recordEvent(&n, ev.EventID, eventMax)
		addActor(&n, ev.ActorID, whoMax)
		n.WhoCount++
		n.UpdatedAt++
		r.byID[id] = n

		return interfaces.MergeApplied, nil
	}

	return interfaces.MergeNoUnreadEntry, nil
}

// recordEvent appends eventID, keeping the newest eventMax (0 = no key).
func recordEvent(n *model.Notification, eventID uint64, eventMax int) {
	if eventID == 0 {
		return
	}
	n.EventIDs = append(n.EventIDs, eventID)
	if len(n.EventIDs) > eventMax {
		n.EventIDs = n.EventIDs[len(n.EventIDs)-eventMax:]
	}
}

// addActor adds actor to who when absent and under the cap.
func addActor(n *model.Notification, actor uint64, whoMax int) {
	for _, w := range n.Who {
		if w == actor {
			return
		}
	}
	if len(n.Who) < whoMax {
		n.Who = append(n.Who, actor)
	}
}

func (r *Notifications) MarkAsReadForUser(_ context.Context, uid, id uint64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.byID[id]
	if !ok || n.UID != uid {
		return false, nil
	}
	n.IsRead = true
	r.byID[id] = n

	return true, nil
}

func (r *Notifications) MarkAllAsReadByUID(_ context.Context, uid uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, n := range r.byID {
		if n.UID == uid {
			n.IsRead = true
			r.byID[id] = n
		}
	}

	return nil
}

func (r *Notifications) CountUnreadByUID(_ context.Context, uid uint64) (int64, error) {
	var c int64
	for _, n := range r.All() {
		if n.UID == uid && !n.IsRead {
			c++
		}
	}

	return c, nil
}

func (r *Notifications) CountByUID(_ context.Context, uid uint64) (int64, error) {
	var c int64
	for _, n := range r.All() {
		if n.UID == uid {
			c++
		}
	}

	return c, nil
}

// ---------------------------------------------------------------- cache

// Cache is an in-memory cache.Cache (TTL ignored).
type Cache struct {
	mu sync.Mutex
	m  map[string]string
}

func NewCache() *Cache { return &Cache{m: map[string]string{}} }

func (c *Cache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	if !ok {
		return "", cache.ErrNotFound
	}

	return v, nil
}

func (c *Cache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch v := value.(type) {
	case string:
		c.m[key] = v
	default:
		c.m[key] = "1"
	}

	return nil
}

func (c *Cache) IncrWithTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, _ := strconv.ParseInt(c.m[key], 10, 64)
	n++
	c.m[key] = strconv.FormatInt(n, 10)

	return n, ttl, nil
}

func (c *Cache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.m, k)
	}

	return nil
}

func (c *Cache) Ping(context.Context) error { return nil }
func (c *Cache) Close() error               { return nil }

// ---------------------------------------------------------------- broker

// Publisher records published messages.
type Publisher struct {
	mu   sync.Mutex
	Msgs []*eventbroker.Message
	Err  error // when set, Publish fails (broker outage)
}

func (p *Publisher) Publish(_ context.Context, msg *eventbroker.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Err != nil {
		return p.Err
	}
	p.Msgs = append(p.Msgs, msg)

	return nil
}

func (p *Publisher) Close() error { return nil }

// Count returns the number of published messages.
func (p *Publisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.Msgs)
}
