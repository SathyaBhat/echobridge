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
	bluesky    *providers.Bluesky
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
