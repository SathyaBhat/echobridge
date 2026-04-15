package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
	"github.com/sathyabhat/echobridge/internal/providers"
)

// seedBlueskyAccount creates a Bluesky account with both access and refresh tokens.
func seedBlueskyAccount(t *testing.T, srv *Server, id string) *models.Account {
	t.Helper()
	a := &models.Account{
		ID:           id,
		Provider:     models.ProviderBluesky,
		DisplayName:  "Bluesky User",
		Username:     "user.bsky.social",
		InstanceURL:  "https://bsky.social",
		AccessToken:  "old-access-" + id,
		RefreshToken: "old-refresh-" + id,
		ChannelID:    "did:plc:" + id,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := srv.db.CreateAccount(a); err != nil {
		t.Fatalf("seedBlueskyAccount: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// refreshBlueskyTokens
// ---------------------------------------------------------------------------

func TestRefreshBlueskyTokens_UpdatesTokensInDB(t *testing.T) {
	srv := newTestServer(t)
	acc := seedBlueskyAccount(t, srv, "bs-1")

	srv.refreshBlueskyTokens()

	got, err := srv.db.GetAccount(acc.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.AccessToken != "jwt-access-refreshed" {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, "jwt-access-refreshed")
	}
	if got.RefreshToken != "jwt-refresh-refreshed" {
		t.Errorf("RefreshToken: got %q, want %q", got.RefreshToken, "jwt-refresh-refreshed")
	}
}

func TestRefreshBlueskyTokens_SkipsNonBluesky(t *testing.T) {
	refreshCalled := false
	mock := &mockBluesky{
		refreshSessionFn: func(pdsURL, refreshJwt string) (*providers.BlueskySessionResponse, error) {
			refreshCalled = true
			return &providers.BlueskySessionResponse{}, nil
		},
	}
	srv := newTestServer(t, withBluesky(mock))
	seedAccount(t, srv.db, "m-1", models.ProviderMastodon)

	srv.refreshBlueskyTokens()

	if refreshCalled {
		t.Error("RefreshSession must not be called for non-Bluesky accounts")
	}
}

func TestRefreshBlueskyTokens_SkipsAccountWithNoRefreshToken(t *testing.T) {
	refreshCalled := false
	mock := &mockBluesky{
		refreshSessionFn: func(pdsURL, refreshJwt string) (*providers.BlueskySessionResponse, error) {
			refreshCalled = true
			return &providers.BlueskySessionResponse{}, nil
		},
	}
	srv := newTestServer(t, withBluesky(mock))
	if err := srv.db.CreateAccount(&models.Account{
		ID:          "bs-no-refresh",
		Provider:    models.ProviderBluesky,
		DisplayName: "No Refresh",
		Username:    "user.bsky.social",
		InstanceURL: "https://bsky.social",
		AccessToken: "some-access",
		// RefreshToken intentionally empty
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	srv.refreshBlueskyTokens()

	if refreshCalled {
		t.Error("RefreshSession must not be called when refresh token is empty")
	}
}

func TestRefreshBlueskyTokens_ContinuesAfterOneFailure(t *testing.T) {
	callCount := 0
	mock := &mockBluesky{
		refreshSessionFn: func(pdsURL, refreshJwt string) (*providers.BlueskySessionResponse, error) {
			callCount++
			if refreshJwt == "old-refresh-bs-fail" {
				return nil, fmt.Errorf("network error")
			}
			return &providers.BlueskySessionResponse{
				AccessJwt:  "jwt-access-refreshed",
				RefreshJwt: "jwt-refresh-refreshed",
			}, nil
		},
	}
	srv := newTestServer(t, withBluesky(mock))
	seedBlueskyAccount(t, srv, "bs-fail")
	acc2 := seedBlueskyAccount(t, srv, "bs-ok")

	srv.refreshBlueskyTokens()

	if callCount != 2 {
		t.Errorf("expected 2 refresh calls, got %d", callCount)
	}
	got, _ := srv.db.GetAccount(acc2.ID)
	if got.AccessToken != "jwt-access-refreshed" {
		t.Errorf("second account not refreshed: token %q", got.AccessToken)
	}
}

// ---------------------------------------------------------------------------
// StartTokenRefresher
// ---------------------------------------------------------------------------

func TestStartTokenRefresher_StopsOnContextCancel(t *testing.T) {
	srv := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		// Use a long interval so the ticker never fires during the test.
		srv.StartTokenRefresher(ctx, time.Hour)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// goroutine exited cleanly
	case <-time.After(time.Second):
		t.Error("StartTokenRefresher did not stop after context cancel")
	}
}
