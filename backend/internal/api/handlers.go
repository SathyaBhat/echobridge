package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sathyabhat/echobridge/internal/models"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.db.ListAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list accounts")
		return
	}
	if accounts == nil {
		accounts = []models.Account{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteAccount(id); err != nil {
		writeError(w, http.StatusNotFound, "Account not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type MastodonAuthRequest struct {
	InstanceURL string `json:"instance_url"`
}

type MastodonAuthResponse struct {
	AuthURL string `json:"auth_url"`
}

var oauthStates = make(map[string]string)

func (s *Server) handleMastodonAuth(w http.ResponseWriter, r *http.Request) {
	var req MastodonAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.InstanceURL == "" {
		writeError(w, http.StatusBadRequest, "Instance URL is required")
		return
	}

	redirectURI := s.baseURL + "/api/accounts/mastodon/callback"

	app, err := s.db.GetMastodonAppByInstance(req.InstanceURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if app == nil {
		app, err = s.mastodon.RegisterApp(req.InstanceURL, redirectURI)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to register app: %v", err))
			return
		}
		app.ID = uuid.New().String()
		if err := s.db.CreateMastodonApp(app); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save app credentials")
			return
		}
	}

	state := generateState()
	oauthStates[state] = app.InstanceURL

	authURL := s.mastodon.GetAuthURL(app.InstanceURL, app.ClientID, redirectURI, state)

	writeJSON(w, http.StatusOK, MastodonAuthResponse{AuthURL: authURL})
}

func (s *Server) handleMastodonCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "Missing code or state", http.StatusBadRequest)
		return
	}

	instanceURL, ok := oauthStates[state]
	if !ok {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	delete(oauthStates, state)

	app, err := s.db.GetMastodonAppByInstance(instanceURL)
	if err != nil || app == nil {
		http.Error(w, "App not found", http.StatusInternalServerError)
		return
	}

	redirectURI := s.baseURL + "/api/accounts/mastodon/callback"

	tokenResp, err := s.mastodon.ExchangeCode(instanceURL, app.ClientID, app.ClientSecret, code, redirectURI)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to exchange code: %v", err), http.StatusBadGateway)
		return
	}

	mastodonAccount, err := s.mastodon.VerifyCredentials(instanceURL, tokenResp.AccessToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to verify credentials: %v", err), http.StatusBadGateway)
		return
	}

	displayName := mastodonAccount.DisplayName
	if displayName == "" {
		displayName = mastodonAccount.Username
	}

	account := &models.Account{
		ID:          uuid.New().String(),
		Provider:    models.ProviderMastodon,
		DisplayName: displayName,
		Username:    mastodonAccount.Acct,
		InstanceURL: instanceURL,
		AccessToken: tokenResp.AccessToken,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.CreateAccount(account); err != nil {
		http.Error(w, "Failed to save account", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, s.pathPrefix+"/profile.html?connected=mastodon", http.StatusFound)
}

func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	mediaID := uuid.New().String()
	ext := filepath.Ext(header.Filename)
	filename := mediaID + ext
	filePath := filepath.Join(s.uploadDir, filename)

	if err := os.MkdirAll(s.uploadDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	dst, err := os.Create(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	media := &models.Media{
		ID:          mediaID,
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Size:        header.Size,
		Path:        filePath,
		CreatedAt:   time.Now(),
	}

	if err := s.db.CreateMedia(media); err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusInternalServerError, "Failed to save media record")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":       media.ID,
		"filename": media.Filename,
	})
}

func (s *Server) handleMediaDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	media, err := s.db.GetMedia(id)
	if err != nil || media == nil {
		writeError(w, http.StatusNotFound, "Media not found")
		return
	}

	os.Remove(media.Path)

	if err := s.db.DeleteMedia(id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete media")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	var req models.PostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "Content is required")
		return
	}

	if len(req.AccountIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one account is required")
		return
	}

	var results []models.PostResult

	for _, accountID := range req.AccountIDs {
		account, err := s.db.GetAccount(accountID)
		if err != nil || account == nil {
			results = append(results, models.PostResult{
				AccountID: accountID,
				Success:   false,
				Error:     "Account not found",
			})
			continue
		}

		var mediaIDs []string
		for _, localMediaID := range req.MediaIDs {
			media, err := s.db.GetMedia(localMediaID)
			if err != nil || media == nil {
				continue
			}

			file, err := os.Open(media.Path)
			if err != nil {
				continue
			}

			remoteID, err := s.mastodon.UploadMedia(r.Context(), account, file, media.Filename, media.ContentType)
			file.Close()
			if err != nil {
				continue
			}
			mediaIDs = append(mediaIDs, remoteID)
		}

		result, err := s.mastodon.Post(r.Context(), account, req.Content, mediaIDs)
		if err != nil {
			results = append(results, models.PostResult{
				AccountID:   account.ID,
				Provider:    string(account.Provider),
				DisplayName: account.DisplayName,
				Success:     false,
				Error:       err.Error(),
			})
		} else {
			results = append(results, *result)
		}
	}

	writeJSON(w, http.StatusOK, models.PostResponse{Results: results})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "window.ECHOBRIDGE_CONFIG = { apiBase: %q };\n", s.pathPrefix+"/api")
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
