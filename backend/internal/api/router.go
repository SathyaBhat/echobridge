package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/sathyabhat/echobridge/internal/db"
	"github.com/sathyabhat/echobridge/internal/providers"
)

type Server struct {
	db         *db.DB
	router     *chi.Mux
	uploadDir  string
	baseURL    string
	pathPrefix string
	mastodon   *providers.Mastodon
}

func NewServer(database *db.DB, uploadDir, baseURL, pathPrefix string) *Server {
	s := &Server{
		db:         database,
		router:     chi.NewRouter(),
		uploadDir:  uploadDir,
		baseURL:    baseURL,
		pathPrefix: pathPrefix,
		mastodon:   providers.NewMastodon(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	r := s.router

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(privateNetworkAccessMiddleware)

	r.Get(s.pathPrefix+"/config.js", s.handleConfig)

	r.Route(s.pathPrefix+"/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		r.Route("/accounts", func(r chi.Router) {
			r.Get("/", s.handleListAccounts)
			r.Delete("/{id}", s.handleDeleteAccount)

			r.Post("/mastodon/auth", s.handleMastodonAuth)
			r.Get("/mastodon/callback", s.handleMastodonCallback)
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

// privateNetworkAccessMiddleware handles Chrome's Private Network Access (PNA)
// preflight checks. When a public origin (e.g. mastodon.social) makes a request
// to a private/local address (e.g. Tailscale 100.x.x.x), Chrome sends an OPTIONS
// preflight with Access-Control-Request-Private-Network: true and requires the
// server to echo back Access-Control-Allow-Private-Network: true.
func privateNetworkAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		next.ServeHTTP(w, r)
	})
}
