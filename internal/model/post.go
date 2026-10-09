package model

import "github.com/aspirit/xsocial/pkg/utils"

// Post is a piece of user content. LikeCount is a denormalised counter kept in
// sync by the post domain and repaired by the worker's reconciliation job.
// LikeVersion grows by one on every change to LikeCount (like, unlike, repair),
// so a repair can be conditioned on "nothing touched the counter since I read
// it" — comparing the value alone would be fooled by a like followed by an
// unlike (ABA).
type Post struct {
	ID          uint64   `bson:"_id" json:"id"`
	UID         uint64   `bson:"uid" json:"uid"`
	Type        PostType `bson:"type" json:"type"`
	Content     string   `bson:"content" json:"content"`
	LikeCount   int64    `bson:"like_count" json:"likeCount"`
	LikeVersion int64    `bson:"like_version" json:"-"`
	CreatedAt   int64    `bson:"created_at" json:"createdAt"`
	UpdatedAt   int64    `bson:"updated_at" json:"updatedAt"`
}

// LikeCounter is a snapshot of a post's counter and its version.
type LikeCounter struct {
	Count   int64
	Version int64
}

// NewPost creates a regular post.
func NewPost(id, uid uint64, content string) *Post {
	now := utils.TimeUtil.Now()

	return &Post{
		ID:        id,
		UID:       uid,
		Type:      PostTypePost,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
