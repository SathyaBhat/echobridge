package db

import (
	"testing"

	"github.com/sathyabhat/echobridge/internal/models"
)

func makeMastodonApp(id, instance string) *models.MastodonApp {
	return &models.MastodonApp{
		ID:           id,
		InstanceURL:  instance,
		ClientID:     "client-" + id,
		ClientSecret: "secret-" + id,
	}
}

func TestCreateAndGetMastodonApp(t *testing.T) {
	db := newTestDB(t)
	app := makeMastodonApp("app-1", "https://mastodon.social")

	if err := db.CreateMastodonApp(app); err != nil {
		t.Fatalf("CreateMastodonApp: %v", err)
	}

	got, err := db.GetMastodonAppByInstance("https://mastodon.social")
	if err != nil {
		t.Fatalf("GetMastodonAppByInstance: %v", err)
	}
	if got == nil {
		t.Fatal("expected app, got nil")
	}
	if got.ClientID != app.ClientID {
		t.Errorf("ClientID: got %q, want %q", got.ClientID, app.ClientID)
	}
	if got.ClientSecret != app.ClientSecret {
		t.Errorf("ClientSecret: got %q, want %q", got.ClientSecret, app.ClientSecret)
	}
}

func TestGetMastodonApp_NotFound(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetMastodonAppByInstance("https://unknown.example")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing app, got %+v", got)
	}
}

func TestCreateMastodonApp_DuplicateInstance(t *testing.T) {
	db := newTestDB(t)

	app := makeMastodonApp("app-dup", "https://mastodon.social")
	if err := db.CreateMastodonApp(app); err != nil {
		t.Fatalf("first CreateMastodonApp: %v", err)
	}

	duplicate := makeMastodonApp("app-dup-2", "https://mastodon.social")
	if err := db.CreateMastodonApp(duplicate); err == nil {
		t.Error("expected error on duplicate instance_url, got nil")
	}
}
