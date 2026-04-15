package db

import (
	"testing"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
)

func makeAccount(id string) *models.Account {
	return &models.Account{
		ID:          id,
		Provider:    models.ProviderMastodon,
		DisplayName: "Alice",
		Username:    "alice",
		InstanceURL: "https://mastodon.social",
		AccessToken: "tok-" + id,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestCreateAndGetAccount(t *testing.T) {
	db := newTestDB(t)
	a := makeAccount("acc-1")

	if err := db.CreateAccount(a); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	got, err := db.GetAccount(a.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got == nil {
		t.Fatal("GetAccount returned nil for existing account")
	}
	if got.ID != a.ID {
		t.Errorf("ID: got %q, want %q", got.ID, a.ID)
	}
	if got.DisplayName != a.DisplayName {
		t.Errorf("DisplayName: got %q, want %q", got.DisplayName, a.DisplayName)
	}
	if got.AccessToken != a.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, a.AccessToken)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetAccount("does-not-exist")
	if err != nil {
		t.Fatalf("GetAccount unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing account, got %+v", got)
	}
}

func TestListAccounts_Empty(t *testing.T) {
	db := newTestDB(t)

	accounts, err := db.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

func TestListAccounts_Multiple(t *testing.T) {
	db := newTestDB(t)

	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		if err := db.CreateAccount(makeAccount(id)); err != nil {
			t.Fatalf("CreateAccount %s: %v", id, err)
		}
	}

	accounts, err := db.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(accounts))
	}
}

func TestDeleteAccount(t *testing.T) {
	db := newTestDB(t)
	a := makeAccount("acc-del")

	if err := db.CreateAccount(a); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := db.DeleteAccount(a.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	got, err := db.GetAccount(a.ID)
	if err != nil {
		t.Fatalf("GetAccount after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestDeleteAccount_NotFound(t *testing.T) {
	db := newTestDB(t)

	err := db.DeleteAccount("does-not-exist")
	if err == nil {
		t.Error("expected error when deleting non-existent account, got nil")
	}
}

func TestUpdateAccountTokens(t *testing.T) {
	db := newTestDB(t)
	a := makeAccount("acc-tok")
	a.RefreshToken = "old-refresh"

	if err := db.CreateAccount(a); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	if err := db.UpdateAccountTokens(a.ID, "new-access", "new-refresh"); err != nil {
		t.Fatalf("UpdateAccountTokens: %v", err)
	}

	got, err := db.GetAccount(a.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.AccessToken != "new-access" {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, "new-access")
	}
	if got.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken: got %q, want %q", got.RefreshToken, "new-refresh")
	}
}
