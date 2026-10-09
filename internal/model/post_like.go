package model

import (
	"fmt"

	"github.com/aspirit/xsocial/pkg/utils"
)

// PostLike records that UID liked PostID. The deterministic _id
// ("<postID>:<uid>") makes like/unlike idempotent via upsert without a
// separate unique index or a check-then-act race.
type PostLike struct {
	ID        string `bson:"_id" json:"id"`
	PostID    uint64 `bson:"post_id" json:"postId"`
	UID       uint64 `bson:"uid" json:"uid"`
	CreatedAt int64  `bson:"created_at" json:"createdAt"`
}

// PostLikeID builds the deterministic document ID for (postID, uid).
func PostLikeID(postID, uid uint64) string {
	return fmt.Sprintf("%d:%d", postID, uid)
}

// NewPostLike creates a like record.
func NewPostLike(postID, uid uint64) *PostLike {
	return &PostLike{
		ID:        PostLikeID(postID, uid),
		PostID:    postID,
		UID:       uid,
		CreatedAt: utils.TimeUtil.Now(),
	}
}
