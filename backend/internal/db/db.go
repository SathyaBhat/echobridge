package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps *sql.DB. Queries are written with "?" placeholders and rebound
// to "$n" when the backing database is PostgreSQL.
type DB struct {
	*sql.DB
	postgres bool
}

// Postgres reports whether the database is backed by PostgreSQL.
func (db *DB) Postgres() bool { return db.postgres }

func (db *DB) rebind(query string) string {
	if !db.postgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.DB.Exec(db.rebind(query), args...)
}

func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.DB.Query(db.rebind(query), args...)
}

func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.DB.QueryRow(db.rebind(query), args...)
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{DB: sqlDB}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// NewPostgres opens a PostgreSQL database from a connection URL
// (e.g. postgres://user:pass@host:5432/dbname) and runs migrations.
func NewPostgres(url string) (*DB, error) {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{DB: sqlDB, postgres: true}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			display_name TEXT NOT NULL,
			username TEXT NOT NULL,
			instance_url TEXT,
			access_token TEXT NOT NULL,
			refresh_token TEXT,
			channel_id TEXT,
			channel_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS mastodon_apps (
			id TEXT PRIMARY KEY,
			instance_url TEXT UNIQUE NOT NULL,
			client_id TEXT NOT NULL,
			client_secret TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS media (
			id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			path TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			media_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts (created_at DESC)`,
		// Results keep a copy of provider/display_name so history survives
		// an account being disconnected; there is deliberately no FK to accounts.
		`CREATE TABLE IF NOT EXISTS post_results (
			post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			position INTEGER NOT NULL,
			account_id TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			success BOOLEAN NOT NULL,
			post_url TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (post_id, position)
		)`,
	}

	for _, m := range migrations {
		if db.postgres {
			m = strings.ReplaceAll(m, "DATETIME", "TIMESTAMPTZ")
			m = strings.ReplaceAll(m, "INTEGER", "BIGINT")
		}
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}
