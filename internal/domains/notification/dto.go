// Package notification defines DTOs for the notification business logic.
package notification

import "github.com/aspirit/xsocial/internal/model"

// GetNotificationRequest represents a request to get a notification.
type GetNotificationRequest struct {
	ID string `json:"id"`
}

// GetNotificationsRequest represents a request to get notifications list.
type GetNotificationsRequest struct {
	UID    uint64 `json:"uid"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// MarkAsReadRequest represents a request to mark a notification as read.
type MarkAsReadRequest struct {
	ID string `json:"id"`
}

// MarkAllAsReadRequest represents a request to mark all notifications as read.
type MarkAllAsReadRequest struct {
	UID uint64 `json:"uid"`
}

// NotificationObject represents a notification. IDs are strings because 64-bit
// Snowflake IDs exceed JavaScript's safe integer range.
// Who has up to WhoMaxSize actors; WhoCount counts every merged event.
// Display: whoCount==1→first user; whoCount==2→"user 1 and user 2"; whoCount>2→"user 1 and (whoCount-1) others".
type NotificationObject struct {
	ID        string         `json:"id"`
	UID       string         `json:"uid"`
	Who       []string       `json:"who"`      // up to WhoMaxSize actor IDs
	WhoCount  int64          `json:"whoCount"` // Every event merged into this entry
	ActType   int8           `json:"actType"`
	PostType  model.PostType `json:"postType"`
	PostID    string         `json:"postId,omitempty"`
	IsRead    bool           `json:"isRead"`
	CreatedAt int64          `json:"createdAt"` // first event
	UpdatedAt int64          `json:"updatedAt"` // latest merged event
}

// NotificationCollection represents a collection of notifications.
type NotificationCollection struct {
	Notifications []NotificationObject `json:"notifications"`
	Total         int64                `json:"total"`
	UnreadCount   int64                `json:"unreadCount"`
}
