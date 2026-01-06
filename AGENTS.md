# EchoBridge - Agent Instructions

## Project Overview

EchoBridge is a cross-posting app with a Go backend and vanilla HTML/CSS/JS frontend (Pico CSS).

## Commands

```bash
# Build and run locally
cd backend && go build -o echobridge ./cmd/server && ./echobridge

# Run with Docker
docker compose up --build

# Run tests (when added)
cd backend && go test ./...

# Check for errors
cd backend && go build ./...
```

## Project Structure

```
echobridge/
├── backend/
│   ├── cmd/server/main.go       # Entry point
│   ├── internal/
│   │   ├── api/                 # HTTP handlers and router
│   │   ├── db/                  # SQLite database layer
│   │   ├── models/              # Data structures
│   │   └── providers/           # Social platform integrations
├── frontend/
│   ├── index.html               # Compose page
│   ├── profile.html             # Account management
│   ├── css/style.css
│   └── js/app.js, profile.js
├── Dockerfile
└── docker-compose.yml
```

## Remaining Tasks

### Frontend Setup (Phase 1)

1. **Create `frontend/index.html`** - Compose page with:
   - Textarea with live character counter (update on `input` event)
   - File input for media with preview thumbnails
   - Checkboxes for each connected account (fetch from `/api/accounts`)
   - Submit button that POSTs to `/api/posts`
   - Results display showing success/failure per platform

2. **Create `frontend/profile.html`** - Account management with:
   - List of connected accounts (fetch from `/api/accounts`)
   - "Connect Mastodon" button with instance URL input
   - Delete account buttons
   - Connection status indicators

3. **Create `frontend/css/style.css`** - Custom styles for:
   - `.char-count` - Character counter styling
   - `.media-preview` - Grid of thumbnail previews
   - `.hidden` - Display none utility
   - `.result-success` / `.result-error` - Post result styling

4. **Create `frontend/js/app.js`** - Compose page logic:
   - Character counter: `document.getElementById('content').addEventListener('input', ...)`
   - Media preview: Read files with FileReader, display thumbnails
   - Form submit: Upload media first, then POST to `/api/posts`
   - Display results

5. **Create `frontend/js/profile.js`** - Profile page logic:
   - Fetch and display accounts
   - Handle "Connect Mastodon" flow (redirect to OAuth)
   - Handle account deletion

### Mastodon Provider (Phase 2)

1. **Create `backend/internal/providers/provider.go`** - Interface:
   ```go
   type Provider interface {
       Name() string
       Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
       UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
   }
   ```

2. **Create `backend/internal/providers/mastodon.go`**:
   - `RegisterApp(instanceURL string)` - POST to `/api/v1/apps` to get client_id/secret
   - `GetAuthURL(instanceURL, clientID, redirectURI, state string)` - Build OAuth URL
   - `ExchangeCode(instanceURL, clientID, clientSecret, code, redirectURI string)` - POST to `/oauth/token`
   - `Post()` - POST to `/api/v1/statuses`
   - `UploadMedia()` - POST to `/api/v2/media`
   - `VerifyCredentials(instanceURL, token string)` - GET `/api/v1/accounts/verify_credentials`

3. **Implement handlers in `backend/internal/api/handlers.go`**:
   - `handleMastodonAuth`: Accept instance URL, register app (or get cached), return OAuth URL
   - `handleMastodonCallback`: Exchange code for token, fetch user info, save account
   - `handleMediaUpload`: Save file to disk, return media ID
   - `handleCreatePost`: For each selected account, call provider.Post()

### API Endpoints Reference

| Endpoint | Method | Request | Response |
|----------|--------|---------|----------|
| `/api/health` | GET | - | `{"status": "ok"}` |
| `/api/accounts` | GET | - | `[{id, provider, display_name, ...}]` |
| `/api/accounts/{id}` | DELETE | - | 204 No Content |
| `/api/accounts/mastodon/auth` | POST | `{"instance_url": "mastodon.social"}` | `{"auth_url": "https://..."}` |
| `/api/accounts/mastodon/callback` | GET | `?code=...&state=...` | Redirect to `/profile.html` |
| `/api/media/upload` | POST | multipart/form-data | `{"id": "...", "filename": "..."}` |
| `/api/posts` | POST | `{"content": "...", "media_ids": [], "account_ids": []}` | `{"results": [...]}` |

### Mastodon API Reference

- Register app: `POST https://{instance}/api/v1/apps`
- OAuth authorize: `GET https://{instance}/oauth/authorize`
- Token exchange: `POST https://{instance}/oauth/token`
- Verify credentials: `GET https://{instance}/api/v1/accounts/verify_credentials`
- Post status: `POST https://{instance}/api/v1/statuses`
- Upload media: `POST https://{instance}/api/v2/media`

## Code Conventions

- Use standard Go project layout
- Error messages should be user-friendly in API responses
- Never expose tokens in JSON responses (use `json:"-"` tag)
- Frontend uses vanilla JS, no frameworks
- Use Pico CSS semantic HTML (minimal classes needed)
