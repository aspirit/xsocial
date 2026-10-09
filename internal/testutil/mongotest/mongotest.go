// Package mongotest gives integration tests an isolated, throw-away MongoDB
// database with all production indexes applied.
package mongotest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/aspirit/xsocial/internal/storage/mongo/indexes"
)

const (
	codeNoReplicationEnabled = 76 // replSetInitiate on a standalone server
	codeAlreadyInitialized   = 23 // replSetInitiate on an initiated member
	replicaSetName           = "rs0"
)

// NewDB connects to MONGODB_URI_PRIMARY (default mongodb://127.0.0.1:27017),
// authenticating with MONGODB_USERNAME_PRIMARY / MONGODB_PASSWORD_PRIMARY when set
// (the docker-compose stack enables auth; CI containers do not), creates a
// uniquely named database, ensures indexes and drops it on cleanup.
//
// A server started with --replSet but never initiated (how CI runs MongoDB) is
// initiated as a single-node replica set first, so transactions are real there.
// Without a reachable server the test is skipped, unless CI_STRICT_INTEGRATION=1
// (set in CI) in which case it fails.
func NewDB(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("MONGODB_URI_PRIMARY")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Client().ApplyURI(uri).SetServerSelectionTimeout(3 * time.Second)
	if !strings.Contains(uri, "replicaSet=") && !strings.Contains(uri, "directConnection=") {
		// Talk to the one server directly: required to initiate a replica set and
		// harmless for a standalone.
		opts.SetDirect(true)
	}
	if user := os.Getenv("MONGODB_USERNAME_PRIMARY"); user != "" {
		opts.SetAuth(options.Credential{
			Username:   user,
			Password:   os.Getenv("MONGODB_PASSWORD_PRIMARY"),
			AuthSource: "admin",
		})
	}
	client, err := mongo.Connect(opts)
	if err == nil {
		err = client.Ping(ctx, nil)
	}
	if err != nil {
		if os.Getenv("CI_STRICT_INTEGRATION") == "1" {
			t.Fatalf("mongo not reachable at %s: %v", uri, err)
		}
		t.Skipf("skipping: mongo not reachable at %s: %v", uri, err)
	}
	if err := ensureReplicaSet(client, uri); err != nil {
		t.Fatalf("replica set: %v", err)
	}

	db := client.Database(fmt.Sprintf("xs_it_%d", time.Now().UnixNano()))
	// Own deadline: the connect context above may be nearly spent after a
	// replica-set initiation.
	ictx, icancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer icancel()
	if err := indexes.EnsureIndexes(ictx, db); err != nil {
		t.Fatalf("ensure indexes: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	return db
}

// SupportsTransactions reports whether db's server is a replica-set member, i.e.
// multi-document transactions are real rather than Base.WithTransaction's
// standalone fallback.
func SupportsTransactions(t *testing.T, db *mongo.Database) bool {
	t.Helper()
	hello, err := runHello(context.Background(), db.Client())
	if err != nil {
		t.Fatalf("hello: %v", err)
	}
	_, isMember := hello["setName"]

	return isMember
}

// RequireTransactions skips the test on a standalone server, or fails it under
// CI_STRICT_INTEGRATION=1: CI must prove the transactional paths, not skip them.
func RequireTransactions(t *testing.T, db *mongo.Database) {
	t.Helper()
	if SupportsTransactions(t, db) {
		return
	}
	if os.Getenv("CI_STRICT_INTEGRATION") == "1" {
		t.Fatal("MongoDB is a standalone server; CI must run a replica set")
	}
	t.Skip("skipping: standalone MongoDB cannot run transactions (dev stack)")
}

func runHello(ctx context.Context, client *mongo.Client) (bson.M, error) {
	var hello bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return nil, fmt.Errorf("hello: %w", err)
	}

	return hello, nil
}

// ensureReplicaSet initiates a single-node replica set when the server was
// started with --replSet and nobody did yet, then waits for it to be primary.
// A standalone server is left alone.
func ensureReplicaSet(client *mongo.Client, uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hello, err := runHello(ctx, client)
	if err != nil {
		return err
	}
	if _, isMember := hello["setName"]; isMember {
		return waitForPrimary(ctx, client)
	}

	host, err := hostOf(uri)
	if err != nil {
		return err
	}
	cfg := bson.D{
		{Key: "_id", Value: replicaSetName},
		{Key: "members", Value: bson.A{bson.D{{Key: "_id", Value: 0}, {Key: "host", Value: host}}}},
	}
	err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetInitiate", Value: cfg}}).Err()
	var cmdErr mongo.CommandError
	switch {
	case err == nil, errors.As(err, &cmdErr) && cmdErr.Code == codeAlreadyInitialized:
		return waitForPrimary(ctx, client)
	case errors.As(err, &cmdErr) && cmdErr.Code == codeNoReplicationEnabled:
		return nil // standalone: transactions fall back, tests that need them skip
	default:
		return fmt.Errorf("replSetInitiate: %w", err)
	}
}

func waitForPrimary(ctx context.Context, client *mongo.Client) error {
	for {
		hello, err := runHello(ctx, client)
		if err == nil {
			if primary, _ := hello["isWritablePrimary"].(bool); primary {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("replica set did not elect a primary: %w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// hostOf returns the host:port of a single-host MongoDB URI, which is how the
// replica-set member must be advertised so this same client can reach it.
func hostOf(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("cannot derive replica-set host from %q: %w", uri, err)
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":27017"
	}

	return host, nil
}
