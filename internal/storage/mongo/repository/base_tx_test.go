package repository

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestIsTransactionsUnsupported(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"standalone code 20", mongo.CommandError{Code: 20, Message: "Transaction numbers are only allowed on a replica set member or mongos"}, true},
		{"not supported in txn code 263", mongo.CommandError{Code: 263, Message: "x"}, true},
		{"replica set message only", errors.New("Transaction numbers are only allowed on a replica set member or mongos"), true},
		{"unrelated command error", mongo.CommandError{Code: 11000, Message: "duplicate key"}, false},
		{"unrelated error", errors.New("connection reset by peer"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransactionsUnsupported(tt.err); got != tt.want {
				t.Fatalf("isTransactionsUnsupported(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
