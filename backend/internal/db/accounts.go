package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
)

func (db *DB) CreateAccount(account *models.Account) error {
	_, err := db.Exec(`
		INSERT INTO accounts (id, provider, display_name, username, instance_url, access_token, refresh_token, channel_id, channel_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, account.ID, account.Provider, account.DisplayName, account.Username, account.InstanceURL, account.AccessToken, account.RefreshToken, account.ChannelID, account.ChannelName, account.CreatedAt, account.UpdatedAt)
	return err
}

func (db *DB) GetAccount(id string) (*models.Account, error) {
	row := db.QueryRow(`
		SELECT id, provider, display_name, username, instance_url, access_token, refresh_token, channel_id, channel_name, created_at, updated_at
		FROM accounts WHERE id = ?
	`, id)

	var a models.Account
	var instanceURL, refreshToken, channelID, channelName sql.NullString
	err := row.Scan(&a.ID, &a.Provider, &a.DisplayName, &a.Username, &instanceURL, &a.AccessToken, &refreshToken, &channelID, &channelName, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	a.InstanceURL = instanceURL.String
	a.RefreshToken = refreshToken.String
	a.ChannelID = channelID.String
	a.ChannelName = channelName.String
	return &a, nil
}

func (db *DB) ListAccounts() ([]models.Account, error) {
	rows, err := db.Query(`
		SELECT id, provider, display_name, username, instance_url, access_token, refresh_token, channel_id, channel_name, created_at, updated_at
		FROM accounts ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		var instanceURL, refreshToken, channelID, channelName sql.NullString
		if err := rows.Scan(&a.ID, &a.Provider, &a.DisplayName, &a.Username, &instanceURL, &a.AccessToken, &refreshToken, &channelID, &channelName, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.InstanceURL = instanceURL.String
		a.RefreshToken = refreshToken.String
		a.ChannelID = channelID.String
		a.ChannelName = channelName.String
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (db *DB) DeleteAccount(id string) error {
	result, err := db.Exec("DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) UpdateAccountTokens(id, accessToken, refreshToken string) error {
	_, err := db.Exec(`
		UPDATE accounts SET access_token = ?, refresh_token = ?, updated_at = ? WHERE id = ?
	`, accessToken, refreshToken, time.Now(), id)
	return err
}
