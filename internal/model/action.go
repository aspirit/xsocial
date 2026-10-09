package model

// ActionTypes enumerates user actions that produce side effects (notifications,
// counters). Values are part of the event wire format: add new ones, never
// renumber.
type ActionTypes int8

const (
	UserActTypeCreatePost ActionTypes = 1
	UserActTypeLike       ActionTypes = 2
)

// PostType discriminates the kinds of content that share the post storage shape.
type PostType int8

const (
	PostTypePost PostType = 1 // Regular post
)
