// Package wire wires the API service with Google Wire.
//
// wire.go declares the injector; `task gen:wire` generates wire_gen.go (checked
// in, and verified up to date by the pre-commit hook and CI).
package wire

import (
	"github.com/aspirit/xsocial/internal/dep"
	"github.com/aspirit/xsocial/internal/domains/auth"
	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	authhandler "github.com/aspirit/xsocial/services/api/handlers/auth"
	notificationhandler "github.com/aspirit/xsocial/services/api/handlers/notification"
	posthandler "github.com/aspirit/xsocial/services/api/handlers/post"
)

// APIDeps is the fully wired object graph of the API service.
type APIDeps struct {
	*dep.BaseDeps

	AuthBiz   auth.IAuthBiz
	Publisher pkgeventbroker.Publisher // closed by main on shutdown

	AuthHandler         *authhandler.Handler
	PostHandler         *posthandler.Handler
	NotificationHandler *notificationhandler.Handler
}
