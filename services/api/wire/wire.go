//go:build wireinject

package wire

import (
	"github.com/google/wire"

	"github.com/aspirit/xsocial/internal/dep"
	internalwire "github.com/aspirit/xsocial/internal/wire"
	authhandler "github.com/aspirit/xsocial/services/api/handlers/auth"
	notificationhandler "github.com/aspirit/xsocial/services/api/handlers/notification"
	posthandler "github.com/aspirit/xsocial/services/api/handlers/post"
)

// APISet is everything the API process needs.
var APISet = wire.NewSet(
	internalwire.InfraSet,
	internalwire.RepositorySet,
	internalwire.DomainSet,
	internalwire.ProvideEventPublisher,
	authhandler.NewHandler,
	posthandler.NewHandler,
	notificationhandler.NewHandler,
	wire.Struct(new(APIDeps), "*"),
)

// InitializeAPIDeps builds the whole API object graph in one call: either every
// dependency is constructed or an error is returned (no partial graph).
func InitializeAPIDeps(baseDeps *dep.BaseDeps) (*APIDeps, error) {
	wire.Build(APISet)

	return nil, nil
}
