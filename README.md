# EchoBridge

A self-hosted cross-posting application for publishing to multiple social platforms simultaneously, build using [AmpCode](https://ampcode.com/).

## Features

- **Cross-posting**: Write once, publish to multiple platforms
- **Media support**: Attach images and videos to your posts
- **Character counter**: Live character count while composing
- **Account management**: Connect and manage multiple accounts
- **Self-hosted**: Runs on your own server, no third-party dependencies

## Supported Platforms

- [x] Mastodon (any instance)
- [x] Bluesky (via app passwords, with rich text — hashtags and links)
- [ ] Twitter (coming soon)
- [ ] Telegram (coming soon)
- [ ] Discord (coming soon)

## Quick Start

### Prerequisites

- Go 1.21+ (for local development)
- Docker (for containerized deployment)

### Run Locally

```bash
# Clone the repository
git clone https://github.com/sathyabhat/echobridge.git
cd echobridge

# Build and run
make run
```

The app will be available at http://localhost:8080

### Run with Docker

```bash
docker compose up --build
```

### Configuration

Set these environment variables to configure EchoBridge:

| Variable | Default | Description |
|----------|---------|-------------|
| `ECHOBRIDGE_PORT` | `8080` | HTTP server port |
| `ECHOBRIDGE_DB_PATH` | `./data/echobridge.db` | SQLite database path |
| `ECHOBRIDGE_UPLOAD_DIR` | `./data/uploads` | Media upload directory |
| `ECHOBRIDGE_BASE_URL` | `http://localhost:8080` | Public URL for OAuth callbacks |
| `ECHOBRIDGE_FRONTEND_DIR` | `../frontend` | Frontend static files directory |

**Important**: Set `ECHOBRIDGE_BASE_URL` to your server's public URL for OAuth to work correctly.

Example for Tailscale:
```bash
ECHOBRIDGE_BASE_URL="http://myserver.tailnet.ts.net:8080" make run
```

## Usage

### Connect an Account

1. Go to the **Accounts** page
2. Expand the platform you want to connect (e.g., Mastodon)
3. Enter required details (instance URL for Mastodon)
4. Click **Connect** and authorize the app

### Create a Post

1. Go to the **Compose** page
2. Write your message
3. Optionally attach media files
4. Select which accounts to post to
5. Click **Post**

## Project Structure

```
echobridge/
├── backend/
│   ├── cmd/server/          # Application entry point
│   └── internal/
│       ├── api/             # HTTP handlers and routing
│       ├── db/              # SQLite database layer
│       ├── models/          # Data structures
│       └── providers/       # Platform integrations
├── frontend/
│   ├── index.html           # Compose page
│   ├── profile.html         # Account management
│   ├── css/                 # Stylesheets
│   └── js/                  # Frontend JavaScript
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Development

```bash
# Build only
make build

# Build and run
make run

# Clean build artifacts
make clean

# Docker commands
make docker-build
make docker-run
make docker-clean
```

## License

MIT
