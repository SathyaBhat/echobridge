package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/sathyabhat/echobridge/internal/db"
	"github.com/sathyabhat/echobridge/internal/models"
	"github.com/sathyabhat/echobridge/internal/providers"
)

type Server struct {
	db         *db.DB
	router     *chi.Mux
	uploadDir  string
	baseURL    string
	pathPrefix string
	mastodon   MastodonService
	bluesky    BlueskyService
}

func NewServer(database *db.DB, uploadDir, baseURL, pathPrefix string) *Server {
	s := &Server{
		db:         database,
		router:     chi.NewRouter(),
		uploadDir:  uploadDir,
		baseURL:    baseURL,
		pathPrefix: pathPrefix,
			mastodon:   providers.NewMastodon(),
		bluesky:    providers.NewBluesky(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	r := s.router

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	// PNA must be before CORS: the CORS middleware short-circuits OPTIONS requests
	// and returns without calling next, so any middleware registered after it never
	// runs for preflights. PNA needs to add its header on the same OPTIONS response.
	r.Use(privateNetworkAccessMiddleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get(s.pathPrefix+"/config.js", s.handleConfig)

	r.Route(s.pathPrefix+"/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		r.Route("/accounts", func(r chi.Router) {
			r.Get("/", s.handleListAccounts)
			r.Delete("/{id}", s.handleDeleteAccount)

			r.Post("/mastodon/auth", s.handleMastodonAuth)
			r.Get("/mastodon/callback", s.handleMastodonCallback)

			r.Post("/bluesky/connect", s.handleBlueskyConnect)
		})

		r.Route("/media", func(r chi.Router) {
			r.Post("/upload", s.handleMediaUpload)
			r.Delete("/{id}", s.handleMediaDelete)
		})

		r.Post("/posts", s.handleCreatePost)
	})
}

func (s *Server) Router() *chi.Mux {
	return s.router
}

// StartTokenRefresher refreshes Bluesky access tokens on the given interval.
// It runs until ctx is cancelled. Call it in a goroutine.
func (s *Server) StartTokenRefresher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshBlueskyTokens()
		}
	}
}

func (s *Server) refreshBlueskyTokens() {
	accounts, err := s.db.ListAccounts()
	if err != nil {
		log.Printf("token refresh: failed to list accounts: %v", err)
		return
	}
	for i := range accounts {
		a := &accounts[i]
		if a.Provider != models.ProviderBluesky || a.RefreshToken == "" {
			continue
		}
		session, err := s.bluesky.RefreshSession(a.InstanceURL, a.RefreshToken)
		if err != nil {
			log.Printf("token refresh: failed to refresh bluesky account %s (%s): %v", a.ID, a.DisplayName, err)
			continue
		}
		if err := s.db.UpdateAccountTokens(a.ID, session.AccessJwt, session.RefreshJwt); err != nil {
			log.Printf("token refresh: failed to save tokens for account %s: %v", a.ID, err)
		}
	}
}

// privateNetworkAccessMiddleware handles Chrome's Private Network Access (PNA)
// checks. It unconditionally sets Access-Control-Allow-Private-Network: true on
// every response so that Chrome allows cross-origin fetches from public origins
// (e.g. mastodon.social) to private/local addresses (e.g. Tailscale 100.x.x.x).
// This covers both the OPTIONS preflight and the actual request.
func privateNetworkAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		next.ServeHTTP(w, r)
	})
}
