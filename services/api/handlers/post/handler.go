// Package posthandler provides HTTP handlers for posts and likes.
package posthandler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/aspirit/xsocial/internal/domains/auth"
	"github.com/aspirit/xsocial/internal/domains/post"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/http/resp"
)

// Handler handles post requests.
type Handler struct {
	authBiz auth.IAuthBiz
	postBiz post.IPostBiz
}

// NewHandler creates the post handler.
func NewHandler(authBiz auth.IAuthBiz, postBiz post.IPostBiz) *Handler {
	return &Handler{authBiz: authBiz, postBiz: postBiz}
}

// CreateAction handles POST /posts.
//
//	@Summary	Create a post
//	@Tags		post
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request	body		post.CreatePostRequest	true	"Post content"
//	@Success	200		{object}	post.PostObject
//	@Failure	400		{object}	resp.ErrorResponse	"Empty or too long content"
//	@Failure	401		{object}	resp.ErrorResponse	"Unauthorized"
//	@Router		/posts [post]
func (h *Handler) CreateAction(c *gin.Context) {
	uid, _ := h.authBiz.GetCurrentUID(c) // route is behind Required()
	var req post.CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	out, err := h.postBiz.Create(c.Request.Context(), uid, &req)
	resp.Finish(c, err, out)
}

// GetAction handles GET /posts/:id.
//
//	@Summary		Get a post
//	@Description	Served from Redis when warm (cache-aside). likedByMe is set for signed-in viewers.
//	@Tags			post
//	@Produce		json
//	@Param			id	path		string	true	"Post ID"
//	@Success		200	{object}	post.PostObject
//	@Failure		404	{object}	resp.ErrorResponse	"Post not found"
//	@Router			/posts/{id} [get]
func (h *Handler) GetAction(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	viewer, _ := h.authBiz.GetCurrentUID(c)
	out, err := h.postBiz.Get(c.Request.Context(), id, viewer)
	resp.Finish(c, err, out)
}

// FeedAction handles GET /posts (global feed, keyset pagination).
//
//	@Summary	Latest posts
//	@Tags		post
//	@Produce	json
//	@Param		cursor	query		string	false	"nextCursor from the previous page"
//	@Param		limit	query		int		false	"Page size (1-50, default 20)"
//	@Success	200		{object}	post.PostCollection
//	@Router		/posts [get]
func (h *Handler) FeedAction(c *gin.Context) {
	var req post.ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	viewer, _ := h.authBiz.GetCurrentUID(c)
	out, err := h.postBiz.ListLatest(c.Request.Context(), viewer, &req)
	resp.Finish(c, err, out)
}

// UserPostsAction handles GET /users/:id/posts.
//
//	@Summary	A user's posts
//	@Tags		post
//	@Produce	json
//	@Param		id		path		string	true	"Author user ID"
//	@Param		cursor	query		string	false	"nextCursor from the previous page"
//	@Param		limit	query		int		false	"Page size (1-50, default 20)"
//	@Success	200		{object}	post.PostCollection
//	@Router		/users/{id}/posts [get]
func (h *Handler) UserPostsAction(c *gin.Context) {
	author, ok := pathID(c)
	if !ok {
		return
	}
	var req post.ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	viewer, _ := h.authBiz.GetCurrentUID(c)
	out, err := h.postBiz.ListByUser(c.Request.Context(), author, viewer, &req)
	resp.Finish(c, err, out)
}

// LikeAction handles POST /posts/:id/like (idempotent).
//
//	@Summary		Like a post
//	@Description	Idempotent. The first like notifies the author asynchronously via the event broker.
//	@Tags			post
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Post ID"
//	@Success		200	{object}	post.PostObject
//	@Failure		404	{object}	resp.ErrorResponse	"Post not found"
//	@Router			/posts/{id}/like [post]
func (h *Handler) LikeAction(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	uid, _ := h.authBiz.GetCurrentUID(c)
	out, err := h.postBiz.Like(c.Request.Context(), id, uid)
	resp.Finish(c, err, out)
}

// UnlikeAction handles DELETE /posts/:id/like (idempotent).
//
//	@Summary	Unlike a post
//	@Tags		post
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Post ID"
//	@Success	200	{object}	post.PostObject
//	@Failure	404	{object}	resp.ErrorResponse	"Post not found"
//	@Router		/posts/{id}/like [delete]
func (h *Handler) UnlikeAction(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	uid, _ := h.authBiz.GetCurrentUID(c)
	out, err := h.postBiz.Unlike(c.Request.Context(), id, uid)
	resp.Finish(c, err, out)
}

func pathID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Fail(c, post.ErrInvalidParams)

		return 0, false
	}

	return id, true
}
