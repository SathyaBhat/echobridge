package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sathyabhat/echobridge/internal/models"
)

// newMastodonTestServer starts a fake Mastodon instance and returns the server
// and a Mastodon provider wired to call it.
func newMastodonTestServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *Mastodon) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	m := NewMastodonWithClient(srv.Client())
	return srv, m
}

// --- NormalizeInstanceURL ---

func TestNormalizeInstanceURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mastodon.social", "https://mastodon.social"},
		{"https://mastodon.social", "https://mastodon.social"},
		{"http://mastodon.social", "http://mastodon.social"},
		{"  mastodon.social/  ", "https://mastodon.social"},
		{"https://mastodon.social/", "https://mastodon.social"},
	}
	for _, tc := range tests {
		got := NormalizeInstanceURL(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeInstanceURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- GetAuthURL ---

func TestGetAuthURL(t *testing.T) {
	m := NewMastodon()
	got := m.GetAuthURL("https://mastodon.social", "client123", "https://myapp/callback", "state42")

	if !strings.HasPrefix(got, "https://mastodon.social/oauth/authorize?") {
		t.Errorf("unexpected auth URL prefix: %s", got)
	}
	for _, param := range []string{"client_id=client123", "state=state42", "response_type=code"} {
		if !strings.Contains(got, param) {
			t.Errorf("auth URL missing %q: %s", param, got)
		}
	}
}

// --- RegisterApp ---

func TestRegisterApp_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(MastodonAppResponse{
			ID:           "app-id-1",
			ClientID:     "cid-abc",
			ClientSecret: "csec-xyz",
		})
	})

	srv, mastodon := newMastodonTestServer(t, mux)

	app, err := mastodon.RegisterApp(srv.URL, "https://myapp/callback")
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	if app.ClientID != "cid-abc" {
		t.Errorf("ClientID: got %q, want %q", app.ClientID, "cid-abc")
	}
	if app.ClientSecret != "csec-xyz" {
		t.Errorf("ClientSecret: got %q, want %q", app.ClientSecret, "csec-xyz")
	}
	if app.InstanceURL != srv.URL {
		t.Errorf("InstanceURL: got %q, want %q", app.InstanceURL, srv.URL)
	}
}

func TestRegisterApp_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	_, err := mastodon.RegisterApp(srv.URL, "https://myapp/callback")
	if err == nil {
		t.Fatal("expected error from server 500, got nil")
	}
}

// --- ExchangeCode ---

func TestExchangeCode_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "access-tok-123",
			TokenType:   "Bearer",
			Scope:       "read write",
		})
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	tok, err := mastodon.ExchangeCode(srv.URL, "cid", "csec", "authcode", "https://myapp/callback")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok.AccessToken != "access-tok-123" {
		t.Errorf("AccessToken: got %q, want %q", tok.AccessToken, "access-tok-123")
	}
}

func TestExchangeCode_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	_, err := mastodon.ExchangeCode(srv.URL, "cid", "csec", "bad-code", "https://myapp/callback")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- VerifyCredentials ---

func TestVerifyCredentials_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/accounts/verify_credentials", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer my-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(MastodonAccount{
			ID:          "1234",
			Username:    "alice",
			DisplayName: "Alice",
			Acct:        "alice@mastodon.social",
		})
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	acc, err := mastodon.VerifyCredentials(srv.URL, "my-token")
	if err != nil {
		t.Fatalf("VerifyCredentials: %v", err)
	}
	if acc.Username != "alice" {
		t.Errorf("Username: got %q, want %q", acc.Username, "alice")
	}
	if acc.Acct != "alice@mastodon.social" {
		t.Errorf("Acct: got %q, want %q", acc.Acct, "alice@mastodon.social")
	}
}

func TestVerifyCredentials_Unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/accounts/verify_credentials", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	_, err := mastodon.VerifyCredentials(srv.URL, "bad-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Post ---

func mastodonAccount(instanceURL string) *models.Account {
	return &models.Account{
		ID:          "acc-1",
		Provider:    models.ProviderMastodon,
		DisplayName: "Alice",
		Username:    "alice",
		InstanceURL: instanceURL,
		AccessToken: "token-abc",
	}
}

func TestMastodonPost_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/statuses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(StatusResponse{ID: "status-1", URL: "https://mastodon.social/@alice/status-1"})
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	result, err := mastodon.Post(context.Background(), mastodonAccount(srv.URL), "Hello!", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.PostURL != "https://mastodon.social/@alice/status-1" {
		t.Errorf("PostURL: got %q", result.PostURL)
	}
}

func TestMastodonPost_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/statuses", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	result, err := mastodon.Post(context.Background(), mastodonAccount(srv.URL), "Hello!", nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Error("expected failure result")
	}
}

// --- UploadMedia ---

func TestMastodonUploadMedia_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/media", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(MediaResponse{ID: "media-remote-1"})
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	id, err := mastodon.UploadMedia(
		context.Background(),
		mastodonAccount(srv.URL),
		strings.NewReader("fake image bytes"),
		"photo.jpg",
		"image/jpeg",
	)
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if id != "media-remote-1" {
		t.Errorf("returned ID: got %q, want %q", id, "media-remote-1")
	}
}

func TestMastodonUploadMedia_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/media", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unprocessable", http.StatusUnprocessableEntity)
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	_, err := mastodon.UploadMedia(
		context.Background(),
		mastodonAccount(srv.URL),
		strings.NewReader("bytes"),
		"photo.jpg",
		"image/jpeg",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- UploadMedia sends correct Content-Type in multipart ---

func TestMastodonUploadMedia_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/media", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(MediaResponse{ID: "x"})
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	acc := mastodonAccount(srv.URL)
	acc.AccessToken = "bearer-token"
	mastodon.UploadMedia(context.Background(), acc, strings.NewReader("data"), "f.jpg", "image/jpeg") //nolint:errcheck

	if gotAuth != "Bearer bearer-token" {
		t.Errorf("Authorization header: got %q, want %q", gotAuth, "Bearer bearer-token")
	}
}

// --- Post sends status text ---

func TestMastodonPost_SendsContent(t *testing.T) {
	var gotBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/statuses", func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(StatusResponse{ID: "1", URL: "https://example.com/1"})
	})
	srv, mastodon := newMastodonTestServer(t, mux)

	mastodon.Post(context.Background(), mastodonAccount(srv.URL), "Test status text", nil) //nolint:errcheck

	if !strings.Contains(string(gotBody), "Test+status+text") && !strings.Contains(string(gotBody), "Test%20status%20text") && !strings.Contains(string(gotBody), "Test status text") {
		t.Errorf("post body does not contain status text: %s", gotBody)
	}
}
