package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/sathyabhat/echobridge/internal/db"
)

type Server struct {
	db        *db.DB
	router    *chi.Mux
	uploadDir string
}

func NewServer(database *db.DB, uploadDir string) *Server {
	s := &Server{
		db:        database,
		router:    chi.NewRouter(),
		uploadDir: uploadDir,
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
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
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
