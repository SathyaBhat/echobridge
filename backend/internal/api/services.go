package api

import (
	"context"
	"io"

	"github.com/sathyabhat/echobridge/internal/models"
	"github.com/sathyabhat/echobridge/internal/providers"
)

// MastodonService is the subset of providers.Mastodon methods used by the server.
// Defined as an interface so handlers can be tested with a mock.
type MastodonService interface {
	Name() string
	Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
	UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
	RegisterApp(instanceURL, redirectURI string) (*models.MastodonApp, error)
	GetAuthURL(instanceURL, clientID, redirectURI, state string) string
	ExchangeCode(instanceURL, clientID, clientSecret, code, redirectURI string) (*providers.TokenResponse, error)
	VerifyCredentials(instanceURL, accessToken string) (*providers.MastodonAccount, error)
}

// BlueskyService is the subset of providers.Bluesky methods used by the server.
type BlueskyService interface {
	Name() string
	Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
	UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
	CreateSession(pdsURL, identifier, appPassword string) (*providers.BlueskySessionResponse, error)
	RefreshSession(pdsURL, refreshJwt string) (*providers.BlueskySessionResponse, error)
}
