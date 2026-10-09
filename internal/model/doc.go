// Package model defines the MongoDB documents of the application.
//
// Every exported struct in this package is scanned by tools/mongo-codegen, which
// generates type-safe field-name constants (internal/storage/mongo/field) and the
// collection-name table (internal/storage/mongo/collections.go). Run `task gen:mongo`
// after changing a model; CI fails if the generated files are stale.
//
// IDs are 64-bit Snowflake IDs (pkg/idgen) and timestamps are Unix milliseconds.
package model
