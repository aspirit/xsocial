// Package form provides form handling utilities for HTTP requests.
package form

type CursorRequest struct {
	Cursor string `json:"cursor" form:"cursor"`
	Size   int    `json:"size" form:"size"`
}

const (
	MaxPageSize     = 100
	DefaultPageSize = 20
	// CursorEnd represents the end of pagination.
	CursorEnd = ""
)

func (request *CursorRequest) CorrectSize() {
	if request.Size <= 0 || request.Size > MaxPageSize {
		request.Size = DefaultPageSize
	}
}
