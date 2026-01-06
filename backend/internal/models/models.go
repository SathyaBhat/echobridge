package models

import "time"

type Provider string

const (
	ProviderMastodon Provider = "mastodon"
	ProviderTwitter  Provider = "twitter"
	ProviderBluesky  Provider = "bluesky"
	ProviderTelegram Provider = "telegram"
	ProviderDiscord  Provider = "discord"
)

type Account struct {
	ID           string    `json:"id"`
	Provider     Provider  `json:"provider"`
	DisplayName  string    `json:"display_name"`
	Username     string    `json:"username"`
	InstanceURL  string    `json:"instance_url,omitempty"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	ChannelID    string    `json:"channel_id,omitempty"`
	ChannelName  string    `json:"channel_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PostRequest struct {
	Content    string   `json:"content"`
	MediaIDs   []string `json:"media_ids,omitempty"`
	AccountIDs []string `json:"account_ids"`
}

type PostResult struct {
	AccountID   string `json:"account_id"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Success     bool   `json:"success"`
	PostURL     string `json:"post_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

type PostResponse struct {
	Results []PostResult `json:"results"`
}

type Media struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Path        string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

type MastodonApp struct {
	ID           string `json:"id"`
	InstanceURL  string `json:"instance_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}
