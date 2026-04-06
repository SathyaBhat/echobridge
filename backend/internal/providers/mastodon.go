package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/sathyabhat/echobridge/internal/models"
)

type Mastodon struct {
	httpClient *http.Client
}

func NewMastodon() *Mastodon {
	return &Mastodon{
		httpClient: &http.Client{},
	}
}

func (m *Mastodon) Name() string {
	return "mastodon"
}

type MastodonAppResponse struct {
	ID           string `json:"id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func (m *Mastodon) RegisterApp(instanceURL, redirectURI string) (*models.MastodonApp, error) {
	instanceURL = NormalizeInstanceURL(instanceURL)

	data := url.Values{}
	data.Set("client_name", "EchoBridge")
	data.Set("redirect_uris", redirectURI)
	data.Set("scopes", "read write")
	data.Set("website", "https://github.com/sathyabhat/echobridge")

	resp, err := m.httpClient.PostForm(instanceURL+"/api/v1/apps", data)
	if err != nil {
		return nil, fmt.Errorf("failed to register app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to register app: %s - %s", resp.Status, string(body))
	}

	var appResp MastodonAppResponse
	if err := json.NewDecoder(resp.Body).Decode(&appResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &models.MastodonApp{
		ID:           appResp.ID,
		InstanceURL:  instanceURL,
		ClientID:     appResp.ClientID,
		ClientSecret: appResp.ClientSecret,
	}, nil
}

func (m *Mastodon) GetAuthURL(instanceURL, clientID, redirectURI, state string) string {
	instanceURL = NormalizeInstanceURL(instanceURL)

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "read write")
	params.Set("state", state)

	return fmt.Sprintf("%s/oauth/authorize?%s", instanceURL, params.Encode())
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

func (m *Mastodon) ExchangeCode(instanceURL, clientID, clientSecret, code, redirectURI string) (*TokenResponse, error) {
	instanceURL = NormalizeInstanceURL(instanceURL)

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("scope", "read write")

	resp, err := m.httpClient.PostForm(instanceURL+"/oauth/token", data)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to exchange code: %s - %s", resp.Status, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

type MastodonAccount struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Acct        string `json:"acct"`
}

func (m *Mastodon) VerifyCredentials(instanceURL, accessToken string) (*MastodonAccount, error) {
	instanceURL = NormalizeInstanceURL(instanceURL)

	req, err := http.NewRequest("GET", instanceURL+"/api/v1/accounts/verify_credentials", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to verify credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to verify credentials: %s - %s", resp.Status, string(body))
	}

	var account MastodonAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to decode account: %w", err)
	}

	return &account, nil
}

type StatusResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (m *Mastodon) Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error) {
	instanceURL := NormalizeInstanceURL(account.InstanceURL)

	data := url.Values{}
	data.Set("status", content)
	for _, id := range mediaIDs {
		data.Add("media_ids[]", id)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", instanceURL+"/api/v1/statuses", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return &models.PostResult{
			AccountID:   account.ID,
			Provider:    string(account.Provider),
			DisplayName: account.DisplayName,
			Success:     false,
			Error:       err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return &models.PostResult{
			AccountID:   account.ID,
			Provider:    string(account.Provider),
			DisplayName: account.DisplayName,
			Success:     false,
			Error:       fmt.Sprintf("%s - %s", resp.Status, string(body)),
		}, nil
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return &models.PostResult{
			AccountID:   account.ID,
			Provider:    string(account.Provider),
			DisplayName: account.DisplayName,
			Success:     false,
			Error:       "Failed to parse response",
		}, nil
	}

	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     true,
		PostURL:     status.URL,
	}, nil
}

type MediaResponse struct {
	ID string `json:"id"`
}

func (m *Mastodon) UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error) {
	instanceURL := NormalizeInstanceURL(account.InstanceURL)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", instanceURL+"/api/v2/media", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to upload media: %s - %s", resp.Status, string(body))
	}

	var mediaResp MediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&mediaResp); err != nil {
		return "", fmt.Errorf("failed to decode media response: %w", err)
	}

	return mediaResp.ID, nil
}

func NormalizeInstanceURL(instanceURL string) string {
	instanceURL = strings.TrimSpace(instanceURL)
	instanceURL = strings.TrimSuffix(instanceURL, "/")

	if !strings.HasPrefix(instanceURL, "http://") && !strings.HasPrefix(instanceURL, "https://") {
		instanceURL = "https://" + instanceURL
	}

	return instanceURL
}
