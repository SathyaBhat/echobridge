package db

import (
	"strings"
	"time"

	"database/sql"

	"github.com/sathyabhat/echobridge/internal/models"
)

// CreatePost stores a post and its per-account results in one transaction.
func (db *DB) CreatePost(post *models.Post) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(db.rebind(`
		INSERT INTO posts (id, content, media_count, created_at) VALUES (?, ?, ?, ?)
	`), post.ID, post.Content, post.MediaCount, post.CreatedAt); err != nil {
		return err
	}

	for i, r := range post.Results {
		if _, err := tx.Exec(db.rebind(`
			INSERT INTO post_results (post_id, position, account_id, provider, display_name, success, post_url, error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`), post.ID, i, r.AccountID, r.Provider, r.DisplayName, r.Success, r.PostURL, r.Error); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListPosts returns posts newest first. If before is non-nil, only posts
// created strictly before it are returned.
func (db *DB) ListPosts(limit int, before *time.Time) ([]models.Post, error) {
	query := `SELECT id, content, media_count, created_at FROM posts`
	var args []any
	if before != nil {
		query += ` WHERE created_at < ?`
		args = append(args, *before)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []models.Post{}
	index := map[string]int{}
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.Content, &p.MediaCount, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Results = []models.PostResult{}
		index[p.ID] = len(posts)
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return posts, nil
	}

	placeholders := make([]string, 0, len(posts))
	resArgs := make([]any, 0, len(posts))
	for _, p := range posts {
		placeholders = append(placeholders, "?")
		resArgs = append(resArgs, p.ID)
	}
	resRows, err := db.Query(`
		SELECT post_id, account_id, provider, display_name, success, post_url, error
		FROM post_results WHERE post_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY post_id, position
	`, resArgs...)
	if err != nil {
		return nil, err
	}
	defer resRows.Close()

	for resRows.Next() {
		var postID string
		var r models.PostResult
		if err := resRows.Scan(&postID, &r.AccountID, &r.Provider, &r.DisplayName, &r.Success, &r.PostURL, &r.Error); err != nil {
			return nil, err
		}
		i := index[postID]
		posts[i].Results = append(posts[i].Results, r)
	}
	return posts, resRows.Err()
}

func (db *DB) DeletePost(id string) error {
	// ON DELETE CASCADE removes the results (SQLite needs foreign_keys=on, set in New).
	result, err := db.Exec("DELETE FROM posts WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) DeleteAllPosts() error {
	_, err := db.Exec("DELETE FROM posts")
	return err
}
