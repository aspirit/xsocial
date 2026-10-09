//go:build integration

// Package tests contains end-to-end tests that drive the real HTTP router
// against a real MongoDB. The broker is replaced by an in-memory publisher whose
// messages are fed to the real notification consumer, so the full asynchronous
// path (API → event → worker logic → MongoDB → API) is exercised in one process.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/internal/domains/auth"
	"github.com/aspirit/xsocial/internal/domains/notification"
	"github.com/aspirit/xsocial/internal/domains/post"
	"github.com/aspirit/xsocial/internal/storage/mongo/repository"
	"github.com/aspirit/xsocial/internal/testutil/fake"
	"github.com/aspirit/xsocial/internal/testutil/mongotest"
	authhandler "github.com/aspirit/xsocial/services/api/handlers/auth"
	notificationhandler "github.com/aspirit/xsocial/services/api/handlers/notification"
	posthandler "github.com/aspirit/xsocial/services/api/handlers/post"
	"github.com/aspirit/xsocial/services/api/router"
	"github.com/aspirit/xsocial/services/api/wire"
)

type app struct {
	engine   *gin.Engine
	pub      *fake.Publisher
	consumer *notification.NotificationHandler
}

func newApp(t *testing.T) *app {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := mongotest.NewDB(t)
	ids := &fake.IDGen{}
	pub := &fake.Publisher{}

	users := repository.NewUserInfoRepository(db)
	posts := repository.NewPostRepository(db)
	likes := repository.NewPostLikeRepository(db)
	notifs := repository.NewNotificationRepository(db)

	authBiz := auth.NewAuthBiz(users, ids, config.JWTConfig{Key: "e2e-signing-secret-0123456789-abcdefghijklmn"})
	events := notification.NewNotificationEventBiz(pub, ids)
	notifBiz := notification.NewNotificationBiz(notifs, ids, nil)
	postBiz := post.NewPostBiz(posts, likes, events, fake.NewCache(), ids)

	deps := &wire.APIDeps{
		AuthBiz:             authBiz,
		Publisher:           pub,
		AuthHandler:         authhandler.NewHandler(authBiz),
		PostHandler:         posthandler.NewHandler(authBiz, postBiz),
		NotificationHandler: notificationhandler.NewHandler(authBiz, notifBiz),
	}
	engine := gin.New()
	router.Register(engine.Group(router.RouteVersionV1), deps)

	return &app{engine: engine, pub: pub, consumer: notification.NewNotificationHandler(notifBiz)}
}

func (a *app) do(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)

	return w.Code, out
}

func (a *app) signup(t *testing.T, username string) (token, uid string) {
	t.Helper()
	code, body := a.do(t, http.MethodPost, "/v1/auth/signup", "", map[string]string{"username": username, "password": "s3cret-pass"})
	require.Equal(t, http.StatusOK, code, body)

	return body["accessToken"].(string), body["uid"].(string)
}

// drainBroker plays the worker: deliver every published event to the consumer.
func (a *app) drainBroker(t *testing.T) {
	t.Helper()
	for _, msg := range a.pub.Msgs {
		require.NoError(t, a.consumer.Handle(context.Background(), msg))
	}
	a.pub.Msgs = nil
}

func TestE2E_PostLikeNotify(t *testing.T) {
	a := newApp(t)

	aliceToken, _ := a.signup(t, "alice_01")
	bobToken, _ := a.signup(t, "bob_0001")
	carolToken, _ := a.signup(t, "carol_01")

	// Signed-in users cannot sign up again (Forbidden auth mode).
	code, _ := a.do(t, http.MethodPost, "/v1/auth/signup", aliceToken, map[string]string{"username": "alice_02", "password": "s3cret-pass"})
	assert.Equal(t, http.StatusForbidden, code)

	code, created := a.do(t, http.MethodPost, "/v1/posts", aliceToken, map[string]string{"content": "hello xsocial"})
	require.Equal(t, http.StatusOK, code, created)
	postID := created["id"].(string)

	// Writes require authentication.
	code, _ = a.do(t, http.MethodPost, "/v1/posts/"+postID+"/like", "", nil)
	assert.Equal(t, http.StatusUnauthorized, code)

	// Bob likes twice (idempotent), Carol once.
	for range 2 {
		code, _ = a.do(t, http.MethodPost, "/v1/posts/"+postID+"/like", bobToken, nil)
		require.Equal(t, http.StatusOK, code)
	}
	code, liked := a.do(t, http.MethodPost, "/v1/posts/"+postID+"/like", carolToken, nil)
	require.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 2, liked["likeCount"])
	assert.Equal(t, true, liked["likedByMe"])

	// Anonymous read: public, not personalised.
	code, got := a.do(t, http.MethodGet, "/v1/posts/"+postID, "", nil)
	require.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 2, got["likeCount"])
	assert.Equal(t, false, got["likedByMe"])

	// Two logical likes → two events → merged into one notification for Alice.
	assert.Len(t, a.pub.Msgs, 2)
	a.drainBroker(t)

	code, inbox := a.do(t, http.MethodGet, "/v1/notifications", aliceToken, nil)
	require.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 1, inbox["unreadCount"])
	items := inbox["notifications"].([]any)
	require.Len(t, items, 1)
	first := items[0].(map[string]any)
	assert.EqualValues(t, 2, first["whoCount"])

	// Bob cannot mark Alice's notification read; Alice can.
	notifPath := fmt.Sprintf("/v1/notifications/%s/read", first["id"])
	code, _ = a.do(t, http.MethodPost, notifPath, bobToken, nil)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = a.do(t, http.MethodPost, notifPath, aliceToken, nil)
	assert.Equal(t, http.StatusOK, code)
	_, unread := a.do(t, http.MethodGet, "/v1/notifications/unread-count", aliceToken, nil)
	assert.EqualValues(t, 0, unread["unread"])

	// Feed and profile listing.
	code, feed := a.do(t, http.MethodGet, "/v1/posts?limit=10", bobToken, nil)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, feed["posts"].([]any), 1)
}

// TestE2E_RedeliveryAfterReadIsIgnored: the broker hands the like event over
// again after Alice has read her inbox; nothing reappears.
func TestE2E_RedeliveryAfterReadIsIgnored(t *testing.T) {
	a := newApp(t)
	aliceToken, _ := a.signup(t, "alice_01")
	bobToken, _ := a.signup(t, "bob_0001")
	code, created := a.do(t, http.MethodPost, "/v1/posts", aliceToken, map[string]string{"content": "hello"})
	require.Equal(t, http.StatusOK, code, created)
	code, _ = a.do(t, http.MethodPost, "/v1/posts/"+created["id"].(string)+"/like", bobToken, nil)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, a.pub.Msgs, 1)
	redelivery := a.pub.Msgs[0]
	a.drainBroker(t)

	code, _ = a.do(t, http.MethodPost, "/v1/notifications/read-all", aliceToken, nil)
	require.Equal(t, http.StatusOK, code)
	require.NoError(t, a.consumer.Handle(context.Background(), redelivery))

	code, inbox := a.do(t, http.MethodGet, "/v1/notifications", aliceToken, nil)
	require.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 0, inbox["unreadCount"])
	assert.Len(t, inbox["notifications"].([]any), 1)
}

func TestE2E_ErrorContract(t *testing.T) {
	a := newApp(t)
	token, _ := a.signup(t, "alice_01")

	cases := []struct {
		name         string
		method, path string
		token        string
		body         any
		status       int
	}{
		{"duplicate username", http.MethodPost, "/v1/auth/signup", "", map[string]string{"username": "alice_01", "password": "s3cret-pass"}, http.StatusConflict},
		{"bad credentials", http.MethodPost, "/v1/auth/login", "", map[string]string{"username": "alice_01", "password": "nope-nope"}, http.StatusUnauthorized},
		{"empty post", http.MethodPost, "/v1/posts", token, map[string]string{"content": "   "}, http.StatusBadRequest},
		{"unknown post", http.MethodGet, "/v1/posts/123456789", "", nil, http.StatusNotFound},
		{"garbage token", http.MethodGet, "/v1/notifications", "not-a-jwt", nil, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := a.do(t, tc.method, tc.path, tc.token, tc.body)
			assert.Equal(t, tc.status, code, body)
			assert.Contains(t, body, "code", "every error uses the {code, message} envelope")
			assert.Contains(t, body, "message")
		})
	}
}
