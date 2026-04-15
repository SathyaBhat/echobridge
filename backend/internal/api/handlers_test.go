package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sathyabhat/echobridge/internal/db"
	"github.com/sathyabhat/echobridge/internal/models"
	"github.com/sathyabhat/echobridge/internal/providers"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockMastodon struct {
	registerAppFn      func(instanceURL, redirectURI string) (*models.MastodonApp, error)
	getAuthURLFn       func(instanceURL, clientID, redirectURI, state string) string
	exchangeCodeFn     func(instanceURL, clientID, clientSecret, code, redirectURI string) (*providers.TokenResponse, error)
	verifyCredsFn      func(instanceURL, accessToken string) (*providers.MastodonAccount, error)
	postFn             func(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
	uploadMediaFn      func(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
}

func (m *mockMastodon) Name() string { return "mastodon" }

func (m *mockMastodon) RegisterApp(instanceURL, redirectURI string) (*models.MastodonApp, error) {
	if m.registerAppFn != nil {
		return m.registerAppFn(instanceURL, redirectURI)
	}
	return &models.MastodonApp{
		ID:           "app-id",
		InstanceURL:  instanceURL,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}, nil
}

func (m *mockMastodon) GetAuthURL(instanceURL, clientID, redirectURI, state string) string {
	if m.getAuthURLFn != nil {
		return m.getAuthURLFn(instanceURL, clientID, redirectURI, state)
	}
	return fmt.Sprintf("%s/oauth/authorize?client_id=%s&state=%s", instanceURL, clientID, state)
}

func (m *mockMastodon) ExchangeCode(instanceURL, clientID, clientSecret, code, redirectURI string) (*providers.TokenResponse, error) {
	if m.exchangeCodeFn != nil {
		return m.exchangeCodeFn(instanceURL, clientID, clientSecret, code, redirectURI)
	}
	return &providers.TokenResponse{AccessToken: "access-token-mock"}, nil
}

func (m *mockMastodon) VerifyCredentials(instanceURL, accessToken string) (*providers.MastodonAccount, error) {
	if m.verifyCredsFn != nil {
		return m.verifyCredsFn(instanceURL, accessToken)
	}
	return &providers.MastodonAccount{
		ID:          "1",
		Username:    "alice",
		DisplayName: "Alice",
		Acct:        "alice@mastodon.social",
	}, nil
}

func (m *mockMastodon) Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error) {
	if m.postFn != nil {
		return m.postFn(ctx, account, content, mediaIDs)
	}
	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     true,
		PostURL:     "https://mastodon.social/@alice/1",
	}, nil
}

func (m *mockMastodon) UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error) {
	if m.uploadMediaFn != nil {
		return m.uploadMediaFn(ctx, account, file, filename, contentType)
	}
	return "remote-media-id", nil
}

// ---------------------------------------------------------------------------

type mockBluesky struct {
	createSessionFn  func(pdsURL, identifier, appPassword string) (*providers.BlueskySessionResponse, error)
	refreshSessionFn func(pdsURL, refreshJwt string) (*providers.BlueskySessionResponse, error)
	postFn           func(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
	uploadMediaFn    func(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
}

func (b *mockBluesky) Name() string { return "bluesky" }

func (b *mockBluesky) CreateSession(pdsURL, identifier, appPassword string) (*providers.BlueskySessionResponse, error) {
	if b.createSessionFn != nil {
		return b.createSessionFn(pdsURL, identifier, appPassword)
	}
	return &providers.BlueskySessionResponse{
		AccessJwt:   "jwt-access",
		RefreshJwt:  "jwt-refresh",
		Handle:      identifier,
		DID:         "did:plc:mock",
		DisplayName: "Mock User",
	}, nil
}

func (b *mockBluesky) Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error) {
	if b.postFn != nil {
		return b.postFn(ctx, account, content, mediaIDs)
	}
	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     true,
		PostURL:     "https://bsky.app/profile/bob.bsky.social/post/rkey",
	}, nil
}

func (b *mockBluesky) RefreshSession(pdsURL, refreshJwt string) (*providers.BlueskySessionResponse, error) {
	if b.refreshSessionFn != nil {
		return b.refreshSessionFn(pdsURL, refreshJwt)
	}
	return &providers.BlueskySessionResponse{
		AccessJwt:  "jwt-access-refreshed",
		RefreshJwt: "jwt-refresh-refreshed",
	}, nil
}

func (b *mockBluesky) UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error) {
	if b.uploadMediaFn != nil {
		return b.uploadMediaFn(ctx, account, file, filename, contentType)
	}
	return `{"$type":"blob","ref":{"$link":"bafy"},"mimeType":"image/jpeg","size":1}`, nil
}

// ---------------------------------------------------------------------------
// Test server builder
// ---------------------------------------------------------------------------

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func newTestServer(t *testing.T, opts ...func(*Server)) *Server {
	t.Helper()
	database := newTestDB(t)
	s := &Server{
		db:         database,
		router:     chi.NewRouter(),
		uploadDir:  t.TempDir(),
		baseURL:    "http://localhost",
		pathPrefix: "",
		mastodon:   &mockMastodon{},
		bluesky:    &mockBluesky{},
	}
	for _, opt := range opts {
		opt(s)
	}
	s.setupRoutes()
	return s
}

func withMastodon(m MastodonService) func(*Server) {
	return func(s *Server) { s.mastodon = m }
}

func withBluesky(b BlueskyService) func(*Server) {
	return func(s *Server) { s.bluesky = b }
}

// ---------------------------------------------------------------------------
// Helper utilities
// ---------------------------------------------------------------------------

func do(t *testing.T, srv *Server, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decodeJSON: %v (body: %s)", err, rr.Body.String())
	}
}

func seedAccount(t *testing.T, database *db.DB, id string, provider models.Provider) *models.Account {
	t.Helper()
	a := &models.Account{
		ID:          id,
		Provider:    provider,
		DisplayName: "Test User",
		Username:    "testuser",
		InstanceURL: "https://mastodon.social",
		AccessToken: "tok-" + id,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := database.CreateAccount(a); err != nil {
		t.Fatalf("seedAccount: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// handleHealth
// ---------------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/api/health", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["status"] != "ok" {
		t.Errorf(`status field: got %q, want "ok"`, resp["status"])
	}
}

// ---------------------------------------------------------------------------
// handleListAccounts
// ---------------------------------------------------------------------------

func TestHandleListAccounts_Empty(t *testing.T) {
	srv := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/api/accounts/", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var accounts []models.Account
	decodeJSON(t, rr, &accounts)
	if len(accounts) != 0 {
		t.Errorf("expected empty list, got %d accounts", len(accounts))
	}
}

func TestHandleListAccounts_WithData(t *testing.T) {
	srv := newTestServer(t)
	seedAccount(t, srv.db, "acc-1", models.ProviderMastodon)
	seedAccount(t, srv.db, "acc-2", models.ProviderBluesky)

	rr := do(t, srv, http.MethodGet, "/api/accounts/", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var accounts []models.Account
	decodeJSON(t, rr, &accounts)
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(accounts))
	}
	// Tokens must not appear in the JSON response.
	raw := rr.Body.String()
	if strings.Contains(raw, "tok-") {
		t.Error("access token leaked in response body")
	}
}

// ---------------------------------------------------------------------------
// handleDeleteAccount
// ---------------------------------------------------------------------------

func TestHandleDeleteAccount(t *testing.T) {
	srv := newTestServer(t)
	seedAccount(t, srv.db, "del-acc", models.ProviderMastodon)

	rr := do(t, srv, http.MethodDelete, "/api/accounts/del-acc", nil)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNoContent)
	}

	// Should be gone.
	got, _ := srv.db.GetAccount("del-acc")
	if got != nil {
		t.Error("account still exists after delete")
	}
}

func TestHandleDeleteAccount_NotFound(t *testing.T) {
	srv := newTestServer(t)
	rr := do(t, srv, http.MethodDelete, "/api/accounts/does-not-exist", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleMastodonAuth
// ---------------------------------------------------------------------------

func TestHandleMastodonAuth_MissingBody(t *testing.T) {
	srv := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/api/accounts/mastodon/auth", strings.NewReader("not json"))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleMastodonAuth_MissingInstanceURL(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, map[string]string{"instance_url": ""})
	rr := do(t, srv, http.MethodPost, "/api/accounts/mastodon/auth", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleMastodonAuth_Success(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, map[string]string{"instance_url": "mastodon.social"})
	rr := do(t, srv, http.MethodPost, "/api/accounts/mastodon/auth", body)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp MastodonAuthResponse
	decodeJSON(t, rr, &resp)
	if resp.AuthURL == "" {
		t.Error("auth_url is empty")
	}
	if !strings.Contains(resp.AuthURL, "mastodon.social") {
		t.Errorf("auth_url missing instance: %s", resp.AuthURL)
	}
}

func TestHandleMastodonAuth_RegisterAppError(t *testing.T) {
	mock := &mockMastodon{
		registerAppFn: func(instanceURL, redirectURI string) (*models.MastodonApp, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	srv := newTestServer(t, withMastodon(mock))
	body := jsonBody(t, map[string]string{"instance_url": "mastodon.social"})
	rr := do(t, srv, http.MethodPost, "/api/accounts/mastodon/auth", body)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadGateway)
	}
}

// Second call with the same instance URL must reuse the cached app (no second RegisterApp call).
func TestHandleMastodonAuth_ReusesCachedApp(t *testing.T) {
	registerCount := 0
	mock := &mockMastodon{
		registerAppFn: func(instanceURL, redirectURI string) (*models.MastodonApp, error) {
			registerCount++
			return &models.MastodonApp{
				ID:           "app-1",
				InstanceURL:  instanceURL,
				ClientID:     "cid",
				ClientSecret: "csec",
			}, nil
		},
	}
	srv := newTestServer(t, withMastodon(mock))

	for i := 0; i < 3; i++ {
		body := jsonBody(t, map[string]string{"instance_url": "mastodon.social"})
		rr := do(t, srv, http.MethodPost, "/api/accounts/mastodon/auth", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("call %d: status %d", i, rr.Code)
		}
	}
	if registerCount != 1 {
		t.Errorf("RegisterApp called %d times, want 1", registerCount)
	}
}

// ---------------------------------------------------------------------------
// handleBlueskyConnect
// ---------------------------------------------------------------------------

func TestHandleBlueskyConnect_MissingFields(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, map[string]string{"handle": "", "app_password": ""})
	rr := do(t, srv, http.MethodPost, "/api/accounts/bluesky/connect", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBlueskyConnect_Success(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, map[string]string{
		"handle":       "bob.bsky.social",
		"app_password": "xxxx-xxxx-xxxx",
	})
	rr := do(t, srv, http.MethodPost, "/api/accounts/bluesky/connect", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	accounts, _ := srv.db.ListAccounts()
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account in DB, got %d", len(accounts))
	}
	if accounts[0].Provider != models.ProviderBluesky {
		t.Errorf("provider: got %q, want %q", accounts[0].Provider, models.ProviderBluesky)
	}
}

func TestHandleBlueskyConnect_AuthError(t *testing.T) {
	mock := &mockBluesky{
		createSessionFn: func(pdsURL, identifier, appPassword string) (*providers.BlueskySessionResponse, error) {
			return nil, fmt.Errorf("invalid credentials")
		},
	}
	srv := newTestServer(t, withBluesky(mock))
	body := jsonBody(t, map[string]string{
		"handle":       "bob.bsky.social",
		"app_password": "bad-password",
	})
	rr := do(t, srv, http.MethodPost, "/api/accounts/bluesky/connect", body)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadGateway)
	}
}

// ---------------------------------------------------------------------------
// handleMediaUpload
// ---------------------------------------------------------------------------

func TestHandleMediaUpload_Success(t *testing.T) {
	srv := newTestServer(t)
	body, contentType := multipartFile(t, "file", "photo.jpg", "image/jpeg", []byte("fake image data"))

	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["id"] == "" {
		t.Error("id is empty")
	}
	if resp["filename"] != "photo.jpg" {
		t.Errorf("filename: got %q, want %q", resp["filename"], "photo.jpg")
	}

	// Record should be in DB.
	media, err := srv.db.GetMedia(resp["id"])
	if err != nil || media == nil {
		t.Fatalf("media not found in DB: %v", err)
	}
}

func TestHandleMediaUpload_NoFile(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=empty")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleMediaDelete
// ---------------------------------------------------------------------------

func TestHandleMediaDelete(t *testing.T) {
	srv := newTestServer(t)

	// Upload first.
	body, contentType := multipartFile(t, "file", "img.png", "image/png", []byte("data"))
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", rr.Code, rr.Body.String())
	}
	var upload map[string]string
	decodeJSON(t, rr, &upload)

	// Delete.
	rr2 := do(t, srv, http.MethodDelete, "/api/media/"+upload["id"], nil)
	if rr2.Code != http.StatusNoContent {
		t.Errorf("delete status: got %d, want %d", rr2.Code, http.StatusNoContent)
	}

	// Gone from DB.
	m, _ := srv.db.GetMedia(upload["id"])
	if m != nil {
		t.Error("media still in DB after delete")
	}
}

func TestHandleMediaDelete_NotFound(t *testing.T) {
	srv := newTestServer(t)
	rr := do(t, srv, http.MethodDelete, "/api/media/does-not-exist", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleCreatePost
// ---------------------------------------------------------------------------

func TestHandleCreatePost_MissingContent(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, models.PostRequest{AccountIDs: []string{"acc-1"}})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreatePost_NoAccounts(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, models.PostRequest{Content: "Hello!", AccountIDs: []string{}})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreatePost_AccountNotFound(t *testing.T) {
	srv := newTestServer(t)
	body := jsonBody(t, models.PostRequest{
		Content:    "Hello!",
		AccountIDs: []string{"ghost-account"},
	})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp models.PostResponse
	decodeJSON(t, rr, &resp)
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Success {
		t.Error("expected failure for unknown account")
	}
}

func TestHandleCreatePost_MastodonSuccess(t *testing.T) {
	srv := newTestServer(t)
	acc := seedAccount(t, srv.db, "m-acc", models.ProviderMastodon)

	body := jsonBody(t, models.PostRequest{
		Content:    "Hello Mastodon!",
		AccountIDs: []string{acc.ID},
	})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp models.PostResponse
	decodeJSON(t, rr, &resp)
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if !resp.Results[0].Success {
		t.Errorf("expected success, got error: %s", resp.Results[0].Error)
	}
}

func TestHandleCreatePost_BlueskySuccess(t *testing.T) {
	srv := newTestServer(t)
	acc := seedAccount(t, srv.db, "b-acc", models.ProviderBluesky)
	acc.InstanceURL = "https://bsky.social" // stored in DB but we seeded with mastodon.social; fix provider
	// Re-seed with correct provider data.
	srv.db.DeleteAccount(acc.ID) //nolint:errcheck
	acc.Provider = models.ProviderBluesky
	acc.InstanceURL = "https://bsky.social"
	if err := srv.db.CreateAccount(acc); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	body := jsonBody(t, models.PostRequest{
		Content:    "Hello Bluesky!",
		AccountIDs: []string{acc.ID},
	})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp models.PostResponse
	decodeJSON(t, rr, &resp)
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if !resp.Results[0].Success {
		t.Errorf("expected success, got error: %s", resp.Results[0].Error)
	}
}

func TestHandleCreatePost_MultipleAccounts(t *testing.T) {
	srv := newTestServer(t)
	a1 := seedAccount(t, srv.db, "a1", models.ProviderMastodon)
	a2 := seedAccount(t, srv.db, "a2", models.ProviderBluesky)

	body := jsonBody(t, models.PostRequest{
		Content:    "Cross-post!",
		AccountIDs: []string{a1.ID, a2.ID},
	})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp models.PostResponse
	decodeJSON(t, rr, &resp)
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	for _, r := range resp.Results {
		if !r.Success {
			t.Errorf("account %s failed: %s", r.AccountID, r.Error)
		}
	}
}

func TestHandleCreatePost_UnsupportedProvider(t *testing.T) {
	srv := newTestServer(t)
	acc := &models.Account{
		ID:          "twitter-acc",
		Provider:    models.ProviderTwitter,
		DisplayName: "Twitter User",
		Username:    "twitteruser",
		AccessToken: "tok",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := srv.db.CreateAccount(acc); err != nil {
		t.Fatalf("seedAccount: %v", err)
	}

	body := jsonBody(t, models.PostRequest{
		Content:    "Hello!",
		AccountIDs: []string{acc.ID},
	})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp models.PostResponse
	decodeJSON(t, rr, &resp)
	if resp.Results[0].Success {
		t.Error("expected failure for unsupported provider")
	}
}

func TestHandleCreatePost_ProviderPostError(t *testing.T) {
	mock := &mockMastodon{
		postFn: func(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error) {
			return nil, fmt.Errorf("rate limited")
		},
	}
	srv := newTestServer(t, withMastodon(mock))
	acc := seedAccount(t, srv.db, "err-acc", models.ProviderMastodon)

	body := jsonBody(t, models.PostRequest{
		Content:    "Hello!",
		AccountIDs: []string{acc.ID},
	})
	rr := do(t, srv, http.MethodPost, "/api/posts", body)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp models.PostResponse
	decodeJSON(t, rr, &resp)
	if resp.Results[0].Success {
		t.Error("expected failure when provider returns error")
	}
	if !strings.Contains(resp.Results[0].Error, "rate limited") {
		t.Errorf("error message: got %q", resp.Results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// handleConfig
// ---------------------------------------------------------------------------

func TestHandleConfig(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/config.js", nil)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type: got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "ECHOBRIDGE_CONFIG") {
		t.Error("response does not contain ECHOBRIDGE_CONFIG")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func jsonBody(t *testing.T, v interface{}) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return bytes.NewReader(b)
}

func multipartFile(t *testing.T, field, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename)}
	h["Content-Type"] = []string{contentType}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("multipartFile CreatePart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("multipartFile Write: %v", err)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}
