package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sathyabhat/echobridge/internal/api"
	"github.com/sathyabhat/echobridge/internal/db"
)

func main() {
	dbPath := getEnv("ECHOBRIDGE_DB_PATH", "./data/echobridge.db")
	uploadDir := getEnv("ECHOBRIDGE_UPLOAD_DIR", "./data/uploads")
	port := getEnv("ECHOBRIDGE_PORT", "8080")
	frontendDir := getEnv("ECHOBRIDGE_FRONTEND_DIR", "../frontend")
	pathPrefix := getEnv("ECHOBRIDGE_PATH_PREFIX", "")
	baseURL := getEnv("ECHOBRIDGE_BASE_URL", "http://localhost:"+port)

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	var database *db.DB
	var err error
	dbDesc := dbPath
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		database, err = db.NewPostgres(databaseURL)
		dbDesc = "postgres (DATABASE_URL)"
		if err == nil {
			// One-time move from a pre-existing SQLite file; no-op if there is
			// none or if Postgres already has accounts. The file is left in place.
			n, impErr := database.ImportSQLite(dbPath)
			if impErr != nil {
				log.Fatalf("Failed to import existing SQLite data from %s: %v", dbPath, impErr)
			}
			if n > 0 {
				log.Printf("Imported %d rows from SQLite database %s", n, dbPath)
			}
		}
	} else {
		database, err = db.New(dbPath)
	}
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	server := api.NewServer(database, uploadDir, baseURL, pathPrefix)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.StartTokenRefresher(ctx, 60*time.Minute)

	router := server.Router()

	absFrontendDir, _ := filepath.Abs(frontendDir)
	fileServer := http.FileServer(http.Dir(absFrontendDir))
	// Force revalidation so browsers never mix stale JS with new HTML after a deploy.
	// Last-Modified is still sent, so unchanged files cost only a 304.
	fs := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
	if pathPrefix != "" {
		router.Handle(pathPrefix+"/*", http.StripPrefix(pathPrefix, fs))
	} else {
		router.Handle("/*", fs)
	}

	log.Printf("Starting EchoBridge server on :%s", port)
	log.Printf("Frontend served from: %s", absFrontendDir)
	log.Printf("Database: %s", dbDesc)

	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
