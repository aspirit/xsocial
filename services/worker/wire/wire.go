//go:build wireinject

package wire

import (
	"github.com/google/wire"

	"github.com/aspirit/xsocial/internal/dep"
	"github.com/aspirit/xsocial/internal/domains/notification"
	internalwire "github.com/aspirit/xsocial/internal/wire"
)

// WorkerSet is everything the worker process needs. It reuses the same shared
// sets as the API, so both processes build domains the same way.
var WorkerSet = wire.NewSet(
	internalwire.InfraSet,
	internalwire.RepositorySet,
	internalwire.ProvideNotificationMergeConfig,
	notification.NewNotificationBiz,
	notification.NewNotificationHandler,
	wire.Struct(new(WorkerDeps), "*"),
)

// InitializeWorkerDeps builds the worker object graph.
func InitializeWorkerDeps(baseDeps *dep.BaseDeps) (*WorkerDeps, error) {
	wire.Build(WorkerSet)

	return nil, nil
}
