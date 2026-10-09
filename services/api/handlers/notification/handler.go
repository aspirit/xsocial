// Package notificationhandler provides HTTP handlers for the notification inbox.
package notificationhandler

import (
	"github.com/gin-gonic/gin"

	"github.com/aspirit/xsocial/internal/domains/auth"
	"github.com/aspirit/xsocial/internal/domains/notification"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/http/form"
	"github.com/aspirit/xsocial/internal/http/resp"
)

// Handler handles notification requests.
type Handler struct {
	authBiz         auth.IAuthBiz
	notificationBiz notification.INotificationBiz
}

// NewHandler creates the notification handler.
func NewHandler(authBiz auth.IAuthBiz, notificationBiz notification.INotificationBiz) *Handler {
	return &Handler{authBiz: authBiz, notificationBiz: notificationBiz}
}

// ListRequest is offset pagination for the inbox.
type ListRequest struct {
	Size   int `form:"size"`
	Offset int `form:"offset" binding:"min=0"`
}

// UnreadCountResponse is the unread badge payload.
type UnreadCountResponse struct {
	Unread int64 `json:"unread" example:"3"`
}

// ListAction handles GET /notifications.
//
//	@Summary		List notifications
//	@Description	Notifications are produced asynchronously by the worker from broker events and merged per (target, action) within a time window ("A and 3 others liked your post").
//	@Tags			notification
//	@Produce		json
//	@Security		BearerAuth
//	@Param			size	query		int	false	"Page size (default 20, max 100)"
//	@Param			offset	query		int	false	"Offset"
//	@Success		200		{object}	notification.NotificationCollection
//	@Failure		401		{object}	resp.ErrorResponse	"Unauthorized"
//	@Router			/notifications [get]
func (h *Handler) ListAction(c *gin.Context) {
	uid, _ := h.authBiz.GetCurrentUID(c)
	var req ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	if req.Size <= 0 || req.Size > form.MaxPageSize {
		req.Size = form.DefaultPageSize
	}
	out, err := h.notificationBiz.GetNotificationsByUserID(c.Request.Context(), uid, req.Size, req.Offset)
	resp.Finish(c, err, out)
}

// UnreadCountAction handles GET /notifications/unread-count.
//
//	@Summary	Unread notification count
//	@Tags		notification
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	UnreadCountResponse
//	@Router		/notifications/unread-count [get]
func (h *Handler) UnreadCountAction(c *gin.Context) {
	uid, _ := h.authBiz.GetCurrentUID(c)
	n, err := h.notificationBiz.GetUnreadCount(c.Request.Context(), uid)
	resp.Finish(c, err, UnreadCountResponse{Unread: n})
}

// MarkReadAction handles POST /notifications/:id/read.
//
//	@Summary	Mark one notification as read
//	@Tags		notification
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Notification ID"
//	@Success	200
//	@Failure	404	{object}	resp.ErrorResponse	"Not found (or not yours)"
//	@Router		/notifications/{id}/read [post]
func (h *Handler) MarkReadAction(c *gin.Context) {
	uid, _ := h.authBiz.GetCurrentUID(c)
	err := h.notificationBiz.MarkAsRead(c.Request.Context(), uid, c.Param("id"))
	resp.Finish(c, err, gin.H{})
}

// MarkAllReadAction handles POST /notifications/read-all.
//
//	@Summary	Mark all notifications as read
//	@Tags		notification
//	@Security	BearerAuth
//	@Success	200
//	@Router		/notifications/read-all [post]
func (h *Handler) MarkAllReadAction(c *gin.Context) {
	uid, _ := h.authBiz.GetCurrentUID(c)
	err := h.notificationBiz.MarkAllAsRead(c.Request.Context(), uid)
	resp.Finish(c, err, gin.H{})
}
