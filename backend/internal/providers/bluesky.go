package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
)

const blueskyDefaultPDS = "https://bsky.social"

type Bluesky struct {
	httpClient *http.Client
}

func NewBluesky() *Bluesky {
	return &Bluesky{
		httpClient: &http.Client{},
	}
}

func (b *Bluesky) Name() string {
	return "bluesky"
}

// --- Session ---

type blueskySessionRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type BlueskySessionResponse struct {
	AccessJwt   string `json:"accessJwt"`
	RefreshJwt  string `json:"refreshJwt"`
	Handle      string `json:"handle"`
	DID         string `json:"did"`
	DisplayName string `json:"displayName,omitempty"`
}

// CreateSession authenticates with an app password and returns session tokens.
func (b *Bluesky) CreateSession(pdsURL, identifier, appPassword string) (*BlueskySessionResponse, error) {
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

	body, err := json.Marshal(blueskySessionRequest{
		Identifier: identifier,
		Password:   appPassword,
	})
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Post(
		pdsURL+"/xrpc/com.atproto.server.createSession",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create session: %s - %s", resp.Status, string(respBody))
	}

	var session BlueskySessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session response: %w", err)
	}

	return &session, nil
}

// --- Post ---

type blueskyCreateRecordRequest struct {
	Repo       string          `json:"repo"`
	Collection string          `json:"collection"`
	Record     json.RawMessage `json:"record"`
}

type blueskyPostRecord struct {
	Type      string         `json:"$type"`
	Text      string         `json:"text"`
	CreatedAt string         `json:"createdAt"`
	Embed     *blueskyEmbed  `json:"embed,omitempty"`
}

type blueskyEmbed struct {
	Type   string          `json:"$type"`
	Images []blueskyImage  `json:"images"`
}

type blueskyImage struct {
	Image blueskyBlob `json:"image"`
	Alt   string      `json:"alt"`
}

type blueskyBlob struct {
	Type     string     `json:"$type"`
	Ref      blobRef    `json:"ref"`
	MimeType string     `json:"mimeType"`
	Size     int64      `json:"size"`
}

type blobRef struct {
	Link string `json:"$link"`
}

type blueskyCreateRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

func (b *Bluesky) Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error) {
	record := blueskyPostRecord{
		Type:      "app.bsky.feed.post",
		Text:      content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// mediaIDs are JSON-encoded blueskyBlob values returned by UploadMedia.
	if len(mediaIDs) > 0 {
		var images []blueskyImage
		for _, blobJSON := range mediaIDs {
			var blob blueskyBlob
			if err := json.Unmarshal([]byte(blobJSON), &blob); err == nil {
				images = append(images, blueskyImage{Image: blob, Alt: ""})
			}
		}
		if len(images) > 0 {
			record.Embed = &blueskyEmbed{
				Type:   "app.bsky.embed.images",
				Images: images,
			}
		}
	}

	recordJSON, err := json.Marshal(record)
	if err != nil {
		return failResult(account, "failed to marshal post record"), nil
	}

	pdsURL := account.InstanceURL
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

	// DID is stored in ChannelID (see handleBlueskyConnect).
	reqBody, err := json.Marshal(blueskyCreateRecordRequest{
		Repo:       account.ChannelID,
		Collection: "app.bsky.feed.post",
		Record:     recordJSON,
	})
	if err != nil {
		return failResult(account, "failed to marshal create record request"), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pdsURL+"/xrpc/com.atproto.repo.createRecord", bytes.NewReader(reqBody))
	if err != nil {
		return failResult(account, err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return failResult(account, err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return failResult(account, fmt.Sprintf("%s - %s", resp.Status, string(respBody))), nil
	}

	var createResp blueskyCreateRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return failResult(account, "failed to parse response"), nil
	}

	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     true,
		PostURL:     atURIToURL(createResp.URI, account.Username),
	}, nil
}

// --- Media upload ---

type blueskyUploadBlobResponse struct {
	Blob blueskyBlob `json:"blob"`
}

// UploadMedia uploads a file to the Bluesky PDS and returns a JSON-encoded
// blob reference that Post() embeds directly in the record.
func (b *Bluesky) UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error) {
	pdsURL := account.InstanceURL
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pdsURL+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", contentType)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to upload blob: %s - %s", resp.Status, string(respBody))
	}

	var uploadResp blueskyUploadBlobResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	// Return the blob as JSON so Post() can reconstruct it for the embed.
	blobJSON, err := json.Marshal(uploadResp.Blob)
	if err != nil {
		return "", fmt.Errorf("failed to marshal blob: %w", err)
	}

	return string(blobJSON), nil
}

// --- Helpers ---

// atURIToURL converts an AT URI (at://did/app.bsky.feed.post/rkey) to a
// bsky.app web URL using the account handle.
func atURIToURL(uri, handle string) string {
	// at://did:plc:xxx/app.bsky.feed.post/rkey
	parts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if len(parts) != 3 {
		return uri
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handle, parts[2])
}

func failResult(account *models.Account, errMsg string) *models.PostResult {
	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     false,
		Error:       errMsg,
	}
}
