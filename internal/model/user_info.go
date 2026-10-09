package model

import "github.com/aspirit/xsocial/pkg/utils"

// UserInfo is an account that can sign in with username + password.
type UserInfo struct {
	ID           uint64 `bson:"_id" json:"id"`
	Username     string `bson:"username" json:"username"`
	DisplayName  string `bson:"display_name" json:"displayName"`
	PasswordHash string `bson:"password_hash" json:"-"`
	CreatedAt    int64  `bson:"created_at" json:"createdAt"`
	UpdatedAt    int64  `bson:"updated_at" json:"updatedAt"`
}

// NewUserInfo creates a user with the given Snowflake ID and bcrypt hash.
func NewUserInfo(id uint64, username, passwordHash string) *UserInfo {
	now := utils.TimeUtil.Now()

	return &UserInfo{
		ID:           id,
		Username:     username,
		DisplayName:  username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
