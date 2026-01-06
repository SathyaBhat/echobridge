# EchoBridge - Cross-Platform Posting Application

## Overview

EchoBridge is a self-hosted application for cross-posting messages to multiple social platforms simultaneously. It consists of a Go-based REST API backend and a lightweight vanilla HTML/CSS/JavaScript frontend.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────────┐
│   Frontend      │────▶│   Go Backend    │────▶│  Social Platforms   │
│  (HTML/JS/CSS)  │     │   (REST API)    │     │  (Mastodon, etc.)   │
└─────────────────┘     └─────────────────┘     └─────────────────────┘
                               │
                               ▼
                        ┌─────────────────┐
                        │   SQLite DB     │
                        │ (accounts/config)│
                        └─────────────────┘
```

## Components

### Backend (Go)

- **Framework**: Standard library `net/http` with gorilla/mux or chi router
- **Database**: SQLite for storing account configurations
- **File Handling**: Temporary storage for media uploads before posting

### Frontend

- **Stack**: Vanilla HTML, CSS, JavaScript (no framework)
- **Styling**: [Pico CSS](https://picocss.com/) - classless semantic CSS (~2KB)
- **No build step required**

## Features

### 1. Compose Message

- Text input area with live character count
- Character limit warnings per platform (e.g., Mastodon 500, Twitter 280)
- Media attachment support (images, videos)
- Preview of attached media

### 2. Platform Selection

- Checkbox for each configured platform
- Show only platforms with valid account connections
- Per-platform status indicators (connected/disconnected)

### 3. Account Management (Profile Page)

- Add/remove accounts for each platform
- OAuth flow for Mastodon, Twitter, Bluesky
- Bot token + channel selection for Telegram/Discord
- Test connection functionality

### 4. Posting

- Submit to selected platforms simultaneously
- Show per-platform success/failure status
- Error messages for failed posts

## Platform Integration Details

### Mastodon (Initial Implementation)

- **Auth**: OAuth 2.0
- **Flow**: 
  1. User provides instance URL (e.g., `mastodon.social`)
  2. App registers with instance (or uses pre-registered app credentials)
  3. User authorizes via OAuth
  4. Store access token in DB
- **API**: REST API for posting statuses and media
- **Character Limit**: 500 (varies by instance)

### Twitter (Future)

- **Auth**: OAuth 2.0 with PKCE
- **Character Limit**: 280

### Bluesky (Future)

- **Auth**: App password or OAuth
- **Character Limit**: 300

### Telegram (Future)

- **Auth**: Bot token
- **Additional**: Channel/chat selection dropdown

### Discord (Future)

- **Auth**: Bot token or Webhook URL
- **Additional**: Server/channel selection dropdown

## API Endpoints

### Posts

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/posts` | Create new cross-post |
| GET | `/api/posts` | List recent posts (optional) |

### Accounts

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/accounts` | List all configured accounts |
| POST | `/api/accounts/{provider}` | Initiate account connection |
| DELETE | `/api/accounts/{id}` | Remove account |
| GET | `/api/accounts/{provider}/callback` | OAuth callback |

### Media

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/media/upload` | Upload media file |
| DELETE | `/api/media/{id}` | Remove uploaded media |

### Channels (Telegram/Discord)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/accounts/{id}/channels` | List available channels |

## Data Models

### Account

```go
type Account struct {
    ID          string    `json:"id"`
    Provider    string    `json:"provider"`    // mastodon, twitter, etc.
    DisplayName string    `json:"display_name"`
    InstanceURL string    `json:"instance_url"` // for Mastodon
    AccessToken string    `json:"-"`            // never expose
    ChannelID   string    `json:"channel_id"`   // for Telegram/Discord
    CreatedAt   time.Time `json:"created_at"`
}
```

### Post Request

```go
type PostRequest struct {
    Content    string   `json:"content"`
    MediaIDs   []string `json:"media_ids"`
    AccountIDs []string `json:"account_ids"`
}
```

### Post Response

```go
type PostResponse struct {
    Results []PostResult `json:"results"`
}

type PostResult struct {
    AccountID string `json:"account_id"`
    Provider  string `json:"provider"`
    Success   bool   `json:"success"`
    PostURL   string `json:"post_url,omitempty"`
    Error     string `json:"error,omitempty"`
}
```

## File Structure

```
echobridge/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/
│   │   │   ├── handlers.go
│   │   │   ├── middleware.go
│   │   │   └── router.go
│   │   ├── db/
│   │   │   ├── db.go
│   │   │   └── migrations.go
│   │   ├── models/
│   │   │   └── models.go
│   │   └── providers/
│   │       ├── provider.go      # interface
│   │       ├── mastodon.go
│   │       ├── twitter.go       # future
│   │       ├── bluesky.go       # future
│   │       ├── telegram.go      # future
│   │       └── discord.go       # future
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── index.html
│   ├── profile.html
│   ├── css/
│   │   └── style.css
│   └── js/
│       ├── app.js
│       └── profile.js
├── docker-compose.yml
├── Dockerfile
├── SPEC.md
└── README.md
```

## Configuration

Environment variables or config file:

```yaml
server:
  port: 8080
  host: 0.0.0.0

database:
  path: ./data/echobridge.db

media:
  upload_dir: ./data/uploads
  max_size_mb: 50

# Provider-specific (optional, for pre-registered apps)
mastodon:
  client_id: ""      # optional, will register dynamically
  client_secret: ""
```

## Security Considerations

- All tokens encrypted at rest in SQLite
- HTTPS recommended even for homelab (via reverse proxy)
- No authentication on app itself (single-user homelab assumption)
- Could add basic auth or PIN code in future

## Docker Deployment

```yaml
version: '3.8'
services:
  echobridge:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
    environment:
      - ECHOBRIDGE_DB_PATH=/app/data/echobridge.db
```
