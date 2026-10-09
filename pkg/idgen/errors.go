// Package idgen provides ID generation utilities.
package idgen

import "errors"

// ID generation errors.
var (
	// ErrInvalidConfiguration indicates that the generator configuration is invalid.
	ErrInvalidConfiguration = errors.New("invalid generator configuration")

	// ErrInvalidDatacenterID indicates that the datacenter ID is out of range.
	ErrInvalidDatacenterID = errors.New("datacenter ID must be between 0 and 31")

	// ErrInvalidWorkerID indicates that the worker ID is out of range.
	ErrInvalidWorkerID = errors.New("worker ID must be between 0 and 31")

	// ErrIDGenerationFailed indicates that ID generation failed.
	ErrIDGenerationFailed = errors.New("failed to generate ID")
)
