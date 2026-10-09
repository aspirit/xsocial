// Package mongo defines MongoDB-related constants shared by every service:
// logical database names, instance types and client defaults.
//
// Collection names and BSON field constants are not written by hand: they are
// generated from internal/model by tools/mongo-codegen (collections.go, field/).
package mongo

// DBCore is the logical name of the core business database (users, posts, likes,
// notifications). Configuration may map it to another physical name per
// environment (storage.mongodb.database_overrides), e.g. xs_db → xs_test.
const DBCore = "xs_db"
