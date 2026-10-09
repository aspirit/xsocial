// Package router owns the URL space of the API: path constants, route
// registration and the rate-limit policy table that is keyed by those paths.
package router

// RouteVersionV1 is the API version prefix.
const RouteVersionV1 = "/v1"

// Path constants (relative to /v1). Keeping them as constants lets the
// rate-limit policy table and the router reference the same strings.
const (
	APIAuthSignup       = "auth/signup"
	APIAuthLogin        = "auth/login"
	APIAuthRefreshToken = "auth/refresh-token"

	APIPosts       = "posts"
	APIPostByID    = "posts/:id"
	APIPostLike    = "posts/:id/like"
	APIUserPosts   = "users/:id/posts"
	APINotifs      = "notifications"
	APINotifUnread = "notifications/unread-count"
	APINotifRead   = "notifications/:id/read"
	APINotifAll    = "notifications/read-all"
)

// Rate-limit policy names (referenced from configs/*.yaml ratelimit.policies).
const (
	ActionLogin        = "login"
	ActionSignup       = "signup"
	ActionRefreshToken = "refresh_token"
	ActionWrite        = "write"
)
