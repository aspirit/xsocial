package utils_test

import (
	"testing"

	"github.com/aspirit/xsocial/pkg/utils"
)

// TestAccount represents a test account model.
type TestAccount struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// TestUserInfo represents a test user info model.
type TestUserInfo struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func TestInterface(t *testing.T) {
	testCases := map[any]string{
		TestAccount{}:   "test_account",
		&TestAccount{}:  "test_account",
		TestUserInfo{}:  "test_user_info",
		&TestUserInfo{}: "test_user_info",
	}
	for k, v := range testCases {
		name := utils.InterfaceUtil.SnakeName(k)
		if v != name {
			t.Errorf("%T expect %s but is %s", k, v, name)
		}
	}

	type TestCase2 struct {
		Value any
		Name  string
	}
	testCases2 := []TestCase2{
		{[]TestAccount{}, "test_account"},
		{[]*TestAccount{}, "test_account"},
		{[]TestUserInfo{}, "test_user_info"},
		{[]*TestUserInfo{}, "test_user_info"},
	}
	for _, v := range testCases2 {
		name := utils.InterfaceUtil.SnakeName(v.Value)
		if v.Name != name {
			t.Errorf("%T expect %s but is %s", v.Value, v.Name, name)
		}
	}
}
