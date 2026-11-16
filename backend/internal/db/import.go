package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// ImportSQLite copies accounts, mastodon_apps and media from an existing
// SQLite database file into this (PostgreSQL) database. It is meant for the
// one-time move from SQLite to PostgreSQL.
//
// It does nothing (and returns 0, nil) if the file does not exist or if this
// database already has accounts, so it is safe to call on every startup.
// The source file is opened read-only and never modified. All rows are copied
// in a single transaction, and existing rows are left untouched.
func (db *DB) ImportSQLite(path string) (int, error) {
	if !db.postgres {
		return 0, errors.New("ImportSQLite requires a PostgreSQL destination")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var existing int
	if err := db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&existing); err != nil {
		return 0, err
	}
	if existing > 0 {
		return 0, nil
	}

	src, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, err
	}
	defer src.Close()

	tx, err := db.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	total := 0

	rows, err := src.Query(`SELECT id, provider, display_name, username, instance_url, access_token,
		refresh_token, channel_id, channel_name, created_at, updated_at FROM accounts`)
	if err != nil {
		return 0, fmt.Errorf("reading accounts: %w", err)
	}
	for rows.Next() {
		var id, provider, displayName, username, accessToken string
		var instanceURL, refreshToken, channelID, channelName sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &provider, &displayName, &username, &instanceURL, &accessToken,
			&refreshToken, &channelID, &channelName, &createdAt, &updatedAt); err != nil {
			rows.Close()
			return 0, err
		}
		if _, err := tx.Exec(db.rebind(`INSERT INTO accounts
			(id, provider, display_name, username, instance_url, access_token, refresh_token, channel_id, channel_name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`),
			id, provider, displayName, username, instanceURL.String, accessToken,
			refreshToken.String, channelID.String, channelName.String, createdAt, updatedAt); err != nil {
			rows.Close()
			return 0, err
		}
		total++
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	rows, err = src.Query(`SELECT id, instance_url, client_id, client_secret, created_at FROM mastodon_apps`)
	if err != nil {
		return 0, fmt.Errorf("reading mastodon_apps: %w", err)
	}
	for rows.Next() {
		var id, instanceURL, clientID, clientSecret string
		var createdAt time.Time
		if err := rows.Scan(&id, &instanceURL, &clientID, &clientSecret, &createdAt); err != nil {
			rows.Close()
			return 0, err
		}
		if _, err := tx.Exec(db.rebind(`INSERT INTO mastodon_apps (id, instance_url, client_id, client_secret, created_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`),
			id, instanceURL, clientID, clientSecret, createdAt); err != nil {
			rows.Close()
			return 0, err
		}
		total++
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	rows, err = src.Query(`SELECT id, filename, content_type, size, path, created_at FROM media`)
	if err != nil {
		return 0, fmt.Errorf("reading media: %w", err)
	}
	for rows.Next() {
		var id, filename, contentType, p string
		var size int64
		var createdAt time.Time
		if err := rows.Scan(&id, &filename, &contentType, &size, &p, &createdAt); err != nil {
			rows.Close()
			return 0, err
		}
		if _, err := tx.Exec(db.rebind(`INSERT INTO media (id, filename, content_type, size, path, created_at)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`),
			id, filename, contentType, size, p, createdAt); err != nil {
			rows.Close()
			return 0, err
		}
		total++
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}
