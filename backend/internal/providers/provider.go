package providers

import (
	"context"
	"io"

	"github.com/sathyabhat/echobridge/internal/models"
)

type Provider interface {
	Name() string
	Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
	UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
}
