// Package authhandler provides HTTP handlers for sign-up, login and token refresh.
package authhandler

import (
	"github.com/gin-gonic/gin"

	"github.com/aspirit/xsocial/internal/domains/auth"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	"github.com/aspirit/xsocial/internal/http/resp"
)

// Handler handles authentication requests. Handlers stay thin: bind, call the
// domain, map the result — all decisions live in the domain layer.
type Handler struct {
	authBiz auth.IAuthBiz
}

// NewHandler creates the auth handler.
func NewHandler(authBiz auth.IAuthBiz) *Handler {
	return &Handler{authBiz: authBiz}
}

// SignupAction handles POST /auth/signup.
//
//	@Summary		Sign up
//	@Description	Create an account with username + password and receive a token pair.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		auth.SignupRequest	true	"Credentials"
//	@Success		200		{object}	auth.TokenResponse
//	@Failure		400		{object}	resp.ErrorResponse	"Invalid parameters"
//	@Failure		409		{object}	resp.ErrorResponse	"Username already exists"
//	@Router			/auth/signup [post]
func (h *Handler) SignupAction(c *gin.Context) {
	var req auth.SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	out, err := h.authBiz.Signup(c.Request.Context(), &req)
	resp.Finish(c, err, out)
}

// LoginAction handles POST /auth/login.
//
//	@Summary		Log in
//	@Description	Exchange username + password for a token pair.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		auth.LoginRequest	true	"Credentials"
//	@Success		200		{object}	auth.TokenResponse
//	@Failure		401		{object}	resp.ErrorResponse	"Invalid credentials"
//	@Router			/auth/login [post]
func (h *Handler) LoginAction(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	out, err := h.authBiz.Login(c.Request.Context(), &req)
	resp.Finish(c, err, out)
}

// RefreshAction handles POST /auth/refresh-token.
//
//	@Summary		Refresh tokens
//	@Description	Exchange a refresh token for a new token pair.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		auth.RefreshRequest	true	"Refresh token"
//	@Success		200		{object}	auth.TokenResponse
//	@Failure		401		{object}	resp.ErrorResponse	"Invalid refresh token"
//	@Router			/auth/refresh-token [post]
func (h *Handler) RefreshAction(c *gin.Context) {
	var req auth.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, internalerr.HandleJSONBindingError(err))

		return
	}
	out, err := h.authBiz.Refresh(c.Request.Context(), &req)
	resp.Finish(c, err, out)
}
