# EchoBridge Queuing System Design

**Date:** 2026-04-20
**Status:** Approved

## Overview

Replace the current synchronous `POST /api/posts` flow (which blocks until all platforms respond) with an async queue. Posts are accepted immediately, delivered in the background by a worker pool, and retried on transient failures. A history section shows all past posts and their per-platform delivery status. Real-time updates are pushed via SSE.

---

## Data Model

Two new SQLite tables, added via the existing migration system in `internal/db/db.go`.

### `post_jobs`

One row per submitted post.

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | UUID |
| `content` | TEXT NOT NULL | Post text |
| `media_ids` | TEXT | JSON array of local media UUIDs |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |
| `status` | TEXT | `pending` \| `processing` \| `partial` \| `completed` \| `failed` |

Overall `status` is derived from deliveries:
- All `delivered` → `completed`
- Mix of `delivered` and `permanent_fail` → `partial`
- All `permanent_fail` (none delivered) → `failed`
- Any still `pending`/`processing` → `processing` or `pending`

### `post_job_deliveries`

One row per (job × account). Tracks per-platform delivery state independently.

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | UUID |
| `job_id` | TEXT | FK → `post_jobs.id` |
| `account_id` | TEXT | FK → `accounts.id` |
| `provider` | TEXT | `mastodon` \| `bluesky` \| ... |
| `display_name` | TEXT | Denormalized for history display |
| `status` | TEXT | `pending` \| `processing` \| `delivered` \| `permanent_fail` |
| `post_url` | TEXT | Set on success |
| `error` | TEXT | Last error message |
| `attempt_count` | INT | Default 0 |
| `next_retry_at` | DATETIME | NULL = eligible immediately |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

---

## Architecture: `internal/queue` Package

### Queue (coordinator)

- Holds a buffered Go channel (`jobs chan string`) of job IDs
- Started at server init alongside the existing `StartTokenRefresher`
- On startup: loads all `pending`/`processing` jobs from DB and re-enqueues them (crash recovery)
- Exposes `Enqueue(jobID string)` for handlers to call after inserting a job row

### Worker Pool

- N goroutines consuming from the jobs channel
- Configurable via `ECHOBRIDGE_WORKERS` env var, default 3
- Per job, a worker:
  1. Sets job status → `processing`
  2. Queries deliveries where `status = 'pending'` AND (`next_retry_at IS NULL` OR `next_retry_at <= now`)
  3. For each eligible delivery, calls the provider (same logic as current `handleCreatePost`)
  4. **On success:** sets delivery `status = delivered`, stores `post_url`
  5. **On transient failure** (5xx, network timeout): increments `attempt_count`; if `attempt_count >= 10` sets `permanent_fail`; otherwise computes `next_retry_at = now + min(2^attempt_count seconds, 7200s)` and sets `status = pending`
  6. **On permanent failure** (4xx: 400, 401, 422, 413, etc.): sets `status = permanent_fail`, stores error — no retry
  7. After all deliveries processed, recomputes and updates overall job status
  8. Publishes SSE event after each delivery state change

### Retry Scheduler

- A background goroutine that ticks every 30 seconds
- Queries for deliveries where `status = 'pending'` AND `next_retry_at <= now`
- Re-enqueues their parent jobs (deduplicating — skip if already in channel)

### Manual Retry

`POST /api/posts/{jobID}/retry`:
- Resets `attempt_count = 0`, `next_retry_at = NULL`, `status = pending` for all `permanent_fail` deliveries of that job
- Re-enqueues the job
- Returns the updated job object

---

## SSE Event Broker (`internal/queue/broker.go`)

A lightweight pub/sub hub injected into workers.

- `Broker` holds a mutex-protected `map[string][]chan Event` — key is job ID or `"*"` for global
- `Subscribe(jobID string) (<-chan Event, unsubscribe func())` — registers a subscriber channel
- `Publish(event Event)` — sends to `"*"` subscribers and job-specific subscribers; **non-blocking** (drops event if channel full, so slow clients can't block workers)
- SSE handlers read from the subscriber channel, write `data: {...}\n\n`, and flush immediately via `http.Flusher`; unsubscribe func called on client disconnect

### SSE Event Shape

```json
{
  "event": "delivery_update",
  "job_id": "uuid",
  "delivery_id": "uuid",
  "provider": "mastodon",
  "display_name": "user@mastodon.social",
  "status": "delivered",
  "post_url": "https://mastodon.social/@user/123",
  "error": null,
  "attempt_count": 1
}
```

---

## API Changes

### Modified Endpoint

| Method | Path | Change |
|--------|------|--------|
| `POST /api/posts` | Returns `{"job_id": "uuid", "status": "pending"}` immediately instead of blocking |

### New Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET /api/posts` | List all post jobs (history), `created_at DESC` |
| `GET /api/posts/{jobID}` | Get single job with all deliveries |
| `POST /api/posts/{jobID}/retry` | Reset `permanent_fail` deliveries and re-enqueue |
| `GET /api/events` | Global SSE stream — all queue activity |
| `GET /api/events/{jobID}` | Per-job SSE stream — updates for one post |

### `GET /api/posts` Response

```json
[
  {
    "id": "uuid",
    "content": "Hello world!",
    "status": "partial",
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:05Z",
    "deliveries": [
      {
        "id": "uuid",
        "account_id": "uuid",
        "provider": "mastodon",
        "display_name": "user@mastodon.social",
        "status": "delivered",
        "post_url": "https://mastodon.social/@user/123",
        "attempt_count": 1,
        "error": null
      },
      {
        "id": "uuid",
        "account_id": "uuid",
        "provider": "bluesky",
        "display_name": "user.bsky.social",
        "status": "permanent_fail",
        "post_url": null,
        "attempt_count": 10,
        "error": "character limit exceeded"
      }
    ]
  }
]
```

---

## Retry Logic

| HTTP Status | Classification | Behavior |
|-------------|---------------|---------|
| 5xx | Transient | Retry with exponential backoff |
| Network timeout / connection refused | Transient | Retry with exponential backoff |
| 400, 401, 403, 413, 422 | Permanent | Mark `permanent_fail`, no retry |
| 429 (rate limit) | Transient | Retry with exponential backoff |

**Backoff formula:** `next_retry_at = now + min(2^attempt_count seconds, 7200s)`

| Attempt | Delay |
|---------|-------|
| 1 | 2s |
| 2 | 4s |
| 3 | 8s |
| 4 | 16s |
| 5 | 32s |
| 6 | 64s |
| 7 | 128s (~2m) |
| 8 | 256s (~4m) |
| 9 | 512s (~8m) |
| 10 | → `permanent_fail` |

Manual retry via `POST /api/posts/{jobID}/retry` resets `attempt_count` to 0 for all `permanent_fail` deliveries and re-enqueues.

---

## Frontend Changes

### Compose Page (`index.html` / `app.js`) — Modified

- `POST /api/posts` returns `job_id` immediately
- Show per-account spinner after submit
- Subscribe to `GET /api/events/{jobID}` SSE
- Update each account row as events arrive:
  - Green checkmark: `delivered`
  - Red X: `permanent_fail`
  - Orange spinner with attempt count: `pending` / retrying
- Show "Post again" button when all deliveries are settled

### History Page (`history.html` / `history.js`) — New

- Fetch `GET /api/posts` on load, render list of post cards
- Each card: content preview, timestamp, per-platform status icons
- Subscribe to `GET /api/events` global SSE for live updates
- "Retry" button on `permanent_fail` delivery rows → `POST /api/posts/{jobID}/retry`
- Add "History" nav link to `index.html` and `profile.html`

---

## New Environment Variable

| Variable | Default | Notes |
|----------|---------|-------|
| `ECHOBRIDGE_WORKERS` | `3` | Number of background worker goroutines |

---

## File Structure Changes

```
backend/
  internal/
    queue/
      queue.go        # Queue struct, worker pool, retry scheduler
      broker.go       # SSE pub/sub broker
    db/
      jobs.go         # post_jobs and post_job_deliveries CRUD
    api/
      handlers.go     # Modified POST /api/posts; new GET /api/posts, retry, SSE handlers
      router.go       # Register new routes

frontend/
  history.html        # New history page
  js/history.js       # New history page logic
  js/app.js           # Modified to handle async job response + SSE
  index.html          # Add History nav link
  profile.html        # Add History nav link
```

---

## Out of Scope

- Pagination for `GET /api/posts` (all history returned; can be added later if needed)
- Post scheduling (submit a post for future delivery)
- Per-user isolation (EchoBridge is single-user/self-hosted)
- Horizontal scaling / distributed workers
