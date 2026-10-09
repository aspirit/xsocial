package post

import (
	"strconv"

	"github.com/aspirit/xsocial/internal/model"
)

// MaxContentRunes caps post length (runes, not bytes, so CJK text is fair).
const MaxContentRunes = 2000

// CreatePostRequest creates a post.
type CreatePostRequest struct {
	Content string `json:"content" binding:"required" example:"Hello, world"`
}

// ListRequest is keyset pagination: pass the previous page's NextCursor.
type ListRequest struct {
	Cursor string `form:"cursor"`
	Limit  int64  `form:"limit,default=20" binding:"min=1,max=50"`
}

// PostObject is the API representation of a post. IDs are strings because
// 64-bit Snowflake IDs exceed JavaScript's safe integer range.
type PostObject struct {
	ID        string `json:"id" example:"93686800198926369"`
	UID       string `json:"uid" example:"93686800198926300"`
	Content   string `json:"content"`
	LikeCount int64  `json:"likeCount"`
	LikedByMe bool   `json:"likedByMe"`
	CreatedAt int64  `json:"createdAt"`
}

// PostCollection is one page of posts.
type PostCollection struct {
	Posts      []PostObject `json:"posts"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

func toObject(p *model.Post, liked bool) PostObject {
	return PostObject{
		ID:        strconv.FormatUint(p.ID, 10),
		UID:       strconv.FormatUint(p.UID, 10),
		Content:   p.Content,
		LikeCount: p.LikeCount,
		LikedByMe: liked,
		CreatedAt: p.CreatedAt,
	}
}
