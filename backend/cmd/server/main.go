package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sathyabhat/echobridge/internal/api"
	"github.com/sathyabhat/echobridge/internal/db"
)

func main() {
	dbPath := getEnv("ECHOBRIDGE_DB_PATH", "./data/echobridge.db")
	uploadDir := getEnv("ECHOBRIDGE_UPLOAD_DIR", "./data/uploads")
	port := getEnv("ECHOBRIDGE_PORT", "8080")
	frontendDir := getEnv("ECHOBRIDGE_FRONTEND_DIR", "../frontend")
	baseURL := getEnv("ECHOBRIDGE_BASE_URL", "http://localhost:"+port)

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	server := api.NewServer(database, uploadDir, baseURL)
	router := server.Router()

	absFrontendDir, _ := filepath.Abs(frontendDir)
	fs := http.FileServer(http.Dir(absFrontendDir))
	router.Handle("/*", fs)

	log.Printf("Starting EchoBridge server on :%s", port)
	log.Printf("Frontend served from: %s", absFrontendDir)
	log.Printf("Database: %s", dbPath)

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
