package router

import (
	"github.com/gin-gonic/gin"

	"github.com/aspirit/xsocial/internal/middleware"
	"github.com/aspirit/xsocial/services/api/wire"
)

// Register mounts every route on v1. Each group declares its auth mode
// explicitly (Forbidden / Optional / Required) instead of checking inside handlers.
func Register(v1 *gin.RouterGroup, deps *wire.APIDeps) {
	authMW := middleware.NewAuthMiddleware(deps.AuthBiz)

	// Guest-only: signed-in users get 403 instead of creating a second session.
	guest := v1.Group("/")
	guest.Use(authMW.Forbidden())
	guest.POST(APIAuthSignup, deps.AuthHandler.SignupAction)
	guest.POST(APIAuthLogin, deps.AuthHandler.LoginAction)

	v1.POST(APIAuthRefreshToken, deps.AuthHandler.RefreshAction)

	// Public reads, personalised (likedByMe) when a valid token is present.
	public := v1.Group("/")
	public.Use(authMW.Optional())
	public.GET(APIPosts, deps.PostHandler.FeedAction)
	public.GET(APIPostByID, deps.PostHandler.GetAction)
	public.GET(APIUserPosts, deps.PostHandler.UserPostsAction)

	// Authenticated writes and the private inbox.
	authed := v1.Group("/")
	authed.Use(authMW.Required())
	authed.POST(APIPosts, deps.PostHandler.CreateAction)
	authed.POST(APIPostLike, deps.PostHandler.LikeAction)
	authed.DELETE(APIPostLike, deps.PostHandler.UnlikeAction)
	authed.GET(APINotifs, deps.NotificationHandler.ListAction)
	authed.GET(APINotifUnread, deps.NotificationHandler.UnreadCountAction)
	authed.POST(APINotifRead, deps.NotificationHandler.MarkReadAction)
	authed.POST(APINotifAll, deps.NotificationHandler.MarkAllReadAction)
}
