package auth

// SignupRequest registers a new account.
type SignupRequest struct {
	Username string `json:"username" binding:"required,min=5,max=20" example:"alice_01"`
	Password string `json:"password" binding:"required,min=6,max=64" example:"s3cret-pass"`
}

// LoginRequest signs in with username + password.
type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"alice_01"`
	Password string `json:"password" binding:"required" example:"s3cret-pass"`
}

// RefreshRequest exchanges a refresh token for a new token pair.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// TokenResponse is returned by signup, login and refresh.
type TokenResponse struct {
	UID              string `json:"uid" example:"93686800198926369"`
	Username         string `json:"username" example:"alice_01"`
	TokenType        string `json:"tokenType" example:"Bearer"`
	AccessToken      string `json:"accessToken"`
	ExpiresAt        int64  `json:"expiresAt"`
	RefreshToken     string `json:"refreshToken"`
	RefreshExpiresAt int64  `json:"refreshExpiresAt"`
}
