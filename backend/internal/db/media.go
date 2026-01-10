package db

import (
	"database/sql"
	"errors"

	"github.com/sathyabhat/echobridge/internal/models"
)

func (db *DB) CreateMedia(media *models.Media) error {
	_, err := db.Exec(`
		INSERT INTO media (id, filename, content_type, size, path, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, media.ID, media.Filename, media.ContentType, media.Size, media.Path, media.CreatedAt)
	return err
}

func (db *DB) GetMedia(id string) (*models.Media, error) {
	row := db.QueryRow(`
		SELECT id, filename, content_type, size, path, created_at
		FROM media WHERE id = ?
	`, id)

	var m models.Media
	err := row.Scan(&m.ID, &m.Filename, &m.ContentType, &m.Size, &m.Path, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) DeleteMedia(id string) error {
	_, err := db.Exec("DELETE FROM media WHERE id = ?", id)
	return err
}
