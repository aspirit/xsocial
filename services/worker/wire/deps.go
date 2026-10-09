// Package wire wires the worker service with Google Wire.
package wire

import (
	"github.com/aspirit/xsocial/internal/dep"
	"github.com/aspirit/xsocial/internal/domains/notification"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
)

// WorkerDeps is the fully wired object graph of the worker service.
type WorkerDeps struct {
	*dep.BaseDeps

	// MessageHandler consumes notification events (implements eventbroker.Handler).
	MessageHandler *notification.NotificationHandler

	// Repositories used by the scheduled jobs.
	Posts  interfaces.IPostRepository
	Likes  interfaces.IPostLikeRepository
	Leases interfaces.IJobLeaseRepository
}
