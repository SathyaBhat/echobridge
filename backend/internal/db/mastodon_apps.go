package db

import (
	"database/sql"
	"errors"

	"github.com/sathyabhat/echobridge/internal/models"
)

func (db *DB) CreateMastodonApp(app *models.MastodonApp) error {
	_, err := db.Exec(`
		INSERT INTO mastodon_apps (id, instance_url, client_id, client_secret)
		VALUES (?, ?, ?, ?)
	`, app.ID, app.InstanceURL, app.ClientID, app.ClientSecret)
	return err
}

func (db *DB) GetMastodonAppByInstance(instanceURL string) (*models.MastodonApp, error) {
	row := db.QueryRow(`
		SELECT id, instance_url, client_id, client_secret
		FROM mastodon_apps WHERE instance_url = ?
	`, instanceURL)

	var app models.MastodonApp
	err := row.Scan(&app.ID, &app.InstanceURL, &app.ClientID, &app.ClientSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app, nil
}
