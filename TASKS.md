# EchoBridge - Task List

## Phase 1: Project Setup & Foundation

- [ ] Initialize Go module (`go mod init github.com/sathyabhat/echobridge`)
- [ ] Create directory structure
- [ ] Set up SQLite database with initial schema
- [ ] Create basic HTTP server with router (chi or gorilla/mux)
- [ ] Add CORS middleware for frontend
- [ ] Create Dockerfile and docker-compose.yml
- [ ] Create basic frontend HTML structure

## Phase 2: Core Backend - Models & Database

- [ ] Define data models (Account, Post, Media)
- [ ] Implement database migrations
- [ ] Create CRUD operations for accounts
- [ ] Implement encrypted token storage (using Go's crypto)

## Phase 3: Provider Interface & Mastodon Implementation

- [ ] Define `Provider` interface
  ```go
  type Provider interface {
      Name() string
      GetAuthURL(state string) string
      ExchangeCode(code string) (*Account, error)
      Post(ctx context.Context, content string, mediaIDs []string) (*PostResult, error)
      UploadMedia(ctx context.Context, file io.Reader, filename string) (string, error)
      ValidateToken(token string) error
  }
  ```
- [ ] Implement Mastodon provider
  - [ ] App registration (dynamic per-instance)
  - [ ] OAuth flow (authorization URL generation)
  - [ ] Token exchange
  - [ ] Post status endpoint
  - [ ] Media upload endpoint
  - [ ] Token validation

## Phase 4: API Endpoints

- [ ] `GET /api/accounts` - List accounts
- [ ] `POST /api/accounts/mastodon/auth` - Start OAuth flow
- [ ] `GET /api/accounts/mastodon/callback` - OAuth callback
- [ ] `DELETE /api/accounts/:id` - Remove account
- [ ] `POST /api/media/upload` - Upload media
- [ ] `DELETE /api/media/:id` - Remove media
- [ ] `POST /api/posts` - Create cross-post

## Phase 5: Frontend - Compose Page

- [ ] Create main layout (index.html)
- [ ] Build compose text area with character counter
- [ ] Add platform-specific character limit indicators
- [ ] Implement media upload UI with drag-and-drop
- [ ] Display media previews with remove option
- [ ] Create platform selection checkboxes
- [ ] Build submit button with loading state
- [ ] Display post results (success/failure per platform)

## Phase 6: Frontend - Profile/Settings Page

- [ ] Create profile page layout (profile.html)
- [ ] List connected accounts with status
- [ ] Add "Connect Account" flow for Mastodon
  - [ ] Instance URL input
  - [ ] OAuth redirect handling
- [ ] Implement account disconnect/remove
- [ ] Add connection test button

## Phase 7: Polish & Testing

- [ ] Error handling and user-friendly messages
- [ ] Loading states throughout UI
- [ ] Mobile-responsive CSS
- [ ] Test with real Mastodon instance
- [ ] Document setup in README

---

## Future Phases (After Mastodon Works)

### Phase 8: Twitter Integration
- [ ] Register Twitter Developer App
- [ ] Implement Twitter OAuth 2.0 with PKCE
- [ ] Implement posting and media upload

### Phase 9: Bluesky Integration
- [ ] Implement Bluesky auth (app password)
- [ ] Implement posting and media upload

### Phase 10: Telegram Integration
- [ ] Bot token input and validation
- [ ] Fetch available chats/channels
- [ ] Channel selection dropdown
- [ ] Implement posting

### Phase 11: Discord Integration
- [ ] Webhook URL or Bot token support
- [ ] Server/channel selection
- [ ] Implement posting

---

## Current Sprint: Phase 1-3 (MVP with Mastodon)

### Immediate Next Steps

1. **Backend skeleton** - Set up Go project, router, SQLite
2. **Mastodon provider** - OAuth + posting
3. **Basic frontend** - Compose + post to Mastodon
4. **Docker setup** - Containerize for homelab

### Definition of Done (MVP)

- [ ] Can connect a Mastodon account via OAuth
- [ ] Can compose a text post
- [ ] Can attach images
- [ ] Can post to connected Mastodon account
- [ ] Can see success/failure result
- [ ] Runs in Docker on homelab
