package db

import (
	"testing"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
)

func makeMedia(id string) *models.Media {
	return &models.Media{
		ID:          id,
		Filename:    "photo-" + id + ".jpg",
		ContentType: "image/jpeg",
		Size:        1024,
		Path:        "/uploads/" + id + ".jpg",
		CreatedAt:   time.Now(),
	}
}

func TestCreateAndGetMedia(t *testing.T) {
	db := newTestDB(t)
	m := makeMedia("media-1")

	if err := db.CreateMedia(m); err != nil {
		t.Fatalf("CreateMedia: %v", err)
	}

	got, err := db.GetMedia(m.ID)
	if err != nil {
		t.Fatalf("GetMedia: %v", err)
	}
	if got == nil {
		t.Fatal("expected media, got nil")
	}
	if got.Filename != m.Filename {
		t.Errorf("Filename: got %q, want %q", got.Filename, m.Filename)
	}
	if got.ContentType != m.ContentType {
		t.Errorf("ContentType: got %q, want %q", got.ContentType, m.ContentType)
	}
	if got.Size != m.Size {
		t.Errorf("Size: got %d, want %d", got.Size, m.Size)
	}
	if got.Path != m.Path {
		t.Errorf("Path: got %q, want %q", got.Path, m.Path)
	}
}

func TestGetMedia_NotFound(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetMedia("does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing media, got %+v", got)
	}
}

func TestDeleteMedia(t *testing.T) {
	db := newTestDB(t)
	m := makeMedia("media-del")

	if err := db.CreateMedia(m); err != nil {
		t.Fatalf("CreateMedia: %v", err)
	}
	if err := db.DeleteMedia(m.ID); err != nil {
		t.Fatalf("DeleteMedia: %v", err)
	}

	got, err := db.GetMedia(m.ID)
	if err != nil {
		t.Fatalf("GetMedia after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
