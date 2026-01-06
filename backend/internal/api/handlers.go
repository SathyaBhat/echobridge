package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func (s *Server) handleMastodonAuth(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "Mastodon auth not yet implemented")
}

func (s *Server) handleMastodonCallback(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "Mastodon callback not yet implemented")
}

func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "Media upload not yet implemented")
}

func (s *Server) handleMediaDelete(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "Media delete not yet implemented")
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "Create post not yet implemented")
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
