// Package post implements posting, reading and liking — the example domain that
// exercises MongoDB (documents + transactions), Redis (cache-aside) and Kafka
// (domain events) together.
package post

import "errors"

// Post domain errors (mapped to API codes by internal/apierrors).
var (
	ErrPostNotFound  = errors.New("post not found")
	ErrContentEmpty  = errors.New("post content is empty")
	ErrContentTooBig = errors.New("post content is too long")
	ErrInvalidParams = errors.New("invalid post parameters")
)
