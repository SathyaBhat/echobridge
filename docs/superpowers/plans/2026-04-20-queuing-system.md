# Queuing System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the synchronous `POST /api/posts` with an async queue that delivers posts in the background, retries transient failures with exponential backoff, and exposes a history API and real-time SSE updates.

**Architecture:** A new `internal/queue` package holds a buffered Go channel, a worker pool (N goroutines), a retry scheduler goroutine, and an SSE broker. All job state persists in two new SQLite tables (`post_jobs`, `post_job_deliveries`). The existing provider logic is reused verbatim inside workers.

**Tech Stack:** Go 1.25.5, chi v5, SQLite3 (mattn/go-sqlite3), google/uuid, Server-Sent Events via `http.Flusher`, Vanilla JS + Pico CSS

---

## File Map

### New files
| File | Responsibility |
|------|---------------|
| `backend/internal/db/jobs.go` | CRUD for `post_jobs` and `post_job_deliveries` |
| `backend/internal/db/jobs_test.go` | DB-layer tests for jobs |
| `backend/internal/models/jobs.go` | `PostJob`, `PostJobDelivery`, `JobStatus`, `DeliveryStatus` types |
| `backend/internal/queue/broker.go` | SSE pub/sub hub — `Broker`, `Event`, `Subscribe`, `Publish` |
| `backend/internal/queue/broker_test.go` | Broker unit tests |
| `backend/internal/queue/queue.go` | `Queue` struct, worker pool, retry scheduler |
| `backend/internal/queue/queue_test.go` | Queue integration tests with in-memory DB |
| `frontend/history.html` | History page HTML |
| `frontend/js/history.js` | History page JS — fetch jobs, SSE, retry button |

### Modified files
| File | Change |
|------|--------|
| `backend/internal/db/db.go` | Add two new table migrations |
| `backend/internal/api/handlers.go` | Rewrite `handleCreatePost`; add `handleListJobs`, `handleGetJob`, `handleRetryJob`, `handleGlobalSSE`, `handleJobSSE` |
| `backend/internal/api/router.go` | Add `Queue *queue.Queue` field; register new routes; pass broker to handlers |
| `backend/internal/api/services.go` | No change needed |
| `backend/cmd/server/main.go` | Instantiate `Queue`, start it, pass to server |
| `frontend/index.html` | Add History nav link; update results section for SSE |
| `frontend/profile.html` | Add History nav link |
| `frontend/js/app.js` | Handle async `job_id` response, subscribe to per-job SSE |
| `frontend/css/style.css` | Add delivery status styles (spinner, checkmark, fail icon) |

---

## Task 1: Add job models

**Files:**
- Create: `backend/internal/models/jobs.go`

- [ ] **Step 1: Create the models file**

```go
// backend/internal/models/jobs.go
package models

import "time"

type JobStatus string
type DeliveryStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusPartial    JobStatus = "partial"
	JobStatusFailed     JobStatus = "failed"
)

const (
	DeliveryStatusPending      DeliveryStatus = "pending"
	DeliveryStatusProcessing   DeliveryStatus = "processing"
	DeliveryStatusDelivered    DeliveryStatus = "delivered"
	DeliveryStatusPermanentFail DeliveryStatus = "permanent_fail"
)

type PostJob struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	MediaIDs  []string       `json:"media_ids,omitempty"`
	Status    JobStatus      `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Deliveries []PostJobDelivery `json:"deliveries,omitempty"`
}

type PostJobDelivery struct {
	ID           string         `json:"id"`
	JobID        string         `json:"job_id"`
	AccountID    string         `json:"account_id"`
	Provider     string         `json:"provider"`
	DisplayName  string         `json:"display_name"`
	Status       DeliveryStatus `json:"status"`
	PostURL      string         `json:"post_url,omitempty"`
	Error        string         `json:"error,omitempty"`
	AttemptCount int            `json:"attempt_count"`
	NextRetryAt  *time.Time     `json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/sathyabhat/code/echobridge/backend && go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/jobs.go
git commit -m "feat: add PostJob and PostJobDelivery models"
```

---

## Task 2: Add DB migrations for job tables

**Files:**
- Modify: `backend/internal/db/db.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/db/jobs_test.go` with just a migration check:

```go
// backend/internal/db/jobs_test.go
package db

import "testing"

func TestMigrate_CreatesJobTables(t *testing.T) {
	db := newTestDB(t)
	for _, table := range []string{"post_jobs", "post_job_deliveries"} {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", table, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/db/ -run TestMigrate_CreatesJobTables -v`
Expected: FAIL — tables do not exist yet

- [ ] **Step 3: Add migrations to db.go**

In `backend/internal/db/db.go`, append two entries to the `migrations` slice inside `migrate()`:

```go
		`CREATE TABLE IF NOT EXISTS post_jobs (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			media_ids TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS post_job_deliveries (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL REFERENCES post_jobs(id),
			account_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			display_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			post_url TEXT,
			error TEXT,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			next_retry_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/db/ -run TestMigrate_CreatesJobTables -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/db/db.go backend/internal/db/jobs_test.go
git commit -m "feat: add post_jobs and post_job_deliveries migrations"
```

---

## Task 3: Implement DB layer for jobs

**Files:**
- Create: `backend/internal/db/jobs.go`
- Modify: `backend/internal/db/jobs_test.go`

- [ ] **Step 1: Write the failing tests**

Replace `backend/internal/db/jobs_test.go` with the full test file:

```go
// backend/internal/db/jobs_test.go
package db

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
)

func TestMigrate_CreatesJobTables(t *testing.T) {
	db := newTestDB(t)
	for _, table := range []string{"post_jobs", "post_job_deliveries"} {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", table, err)
		}
	}
}

func makeJob(id string) *models.PostJob {
	return &models.PostJob{
		ID:        id,
		Content:   "Hello world from " + id,
		MediaIDs:  []string{"media-1"},
		Status:    models.JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func makeDelivery(id, jobID string) *models.PostJobDelivery {
	return &models.PostJobDelivery{
		ID:          id,
		JobID:       jobID,
		AccountID:   "acc-1",
		Provider:    "mastodon",
		DisplayName: "alice@mastodon.social",
		Status:      models.DeliveryStatusPending,
		AttemptCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestCreateAndGetJob(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-1")
	if err := db.CreateJob(j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	got, err := db.GetJob(j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got == nil {
		t.Fatal("GetJob returned nil")
	}
	if got.Content != j.Content {
		t.Errorf("Content: got %q, want %q", got.Content, j.Content)
	}
	if len(got.MediaIDs) != 1 || got.MediaIDs[0] != "media-1" {
		t.Errorf("MediaIDs: got %v, want [media-1]", got.MediaIDs)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	db := newTestDB(t)
	got, err := db.GetJob("no-such-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestListJobs(t *testing.T) {
	db := newTestDB(t)
	for _, id := range []string{"job-a", "job-b", "job-c"} {
		if err := db.CreateJob(makeJob(id)); err != nil {
			t.Fatalf("CreateJob %s: %v", id, err)
		}
	}
	jobs, err := db.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestUpdateJobStatus(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-upd")
	if err := db.CreateJob(j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := db.UpdateJobStatus(j.ID, models.JobStatusCompleted); err != nil {
		t.Fatalf("UpdateJobStatus: %v", err)
	}
	got, _ := db.GetJob(j.ID)
	if got.Status != models.JobStatusCompleted {
		t.Errorf("Status: got %q, want %q", got.Status, models.JobStatusCompleted)
	}
}

func TestCreateAndGetDelivery(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-del-1")
	db.CreateJob(j)

	d := makeDelivery("del-1", j.ID)
	if err := db.CreateDelivery(d); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	got, err := db.GetDelivery(d.ID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if got == nil {
		t.Fatal("GetDelivery returned nil")
	}
	if got.Provider != d.Provider {
		t.Errorf("Provider: got %q, want %q", got.Provider, d.Provider)
	}
}

func TestListDeliveriesByJob(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-listdel")
	db.CreateJob(j)

	for _, id := range []string{"d1", "d2"} {
		db.CreateDelivery(makeDelivery(id, j.ID))
	}

	deliveries, err := db.ListDeliveriesByJob(j.ID)
	if err != nil {
		t.Fatalf("ListDeliveriesByJob: %v", err)
	}
	if len(deliveries) != 2 {
		t.Errorf("expected 2 deliveries, got %d", len(deliveries))
	}
}

func TestUpdateDeliverySuccess(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-ds")
	db.CreateJob(j)
	d := makeDelivery("del-s", j.ID)
	db.CreateDelivery(d)

	if err := db.UpdateDeliverySuccess(d.ID, "https://mastodon.social/@alice/1"); err != nil {
		t.Fatalf("UpdateDeliverySuccess: %v", err)
	}
	got, _ := db.GetDelivery(d.ID)
	if got.Status != models.DeliveryStatusDelivered {
		t.Errorf("Status: got %q, want delivered", got.Status)
	}
	if got.PostURL != "https://mastodon.social/@alice/1" {
		t.Errorf("PostURL: got %q", got.PostURL)
	}
}

func TestUpdateDeliveryRetry(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-dr")
	db.CreateJob(j)
	d := makeDelivery("del-r", j.ID)
	db.CreateDelivery(d)

	next := time.Now().Add(4 * time.Second)
	if err := db.UpdateDeliveryRetry(d.ID, "server error", 1, next); err != nil {
		t.Fatalf("UpdateDeliveryRetry: %v", err)
	}
	got, _ := db.GetDelivery(d.ID)
	if got.Status != models.DeliveryStatusPending {
		t.Errorf("Status: got %q, want pending", got.Status)
	}
	if got.AttemptCount != 1 {
		t.Errorf("AttemptCount: got %d, want 1", got.AttemptCount)
	}
}

func TestUpdateDeliveryPermanentFail(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-dpf")
	db.CreateJob(j)
	d := makeDelivery("del-pf", j.ID)
	db.CreateDelivery(d)

	if err := db.UpdateDeliveryPermanentFail(d.ID, "character limit exceeded", 3); err != nil {
		t.Fatalf("UpdateDeliveryPermanentFail: %v", err)
	}
	got, _ := db.GetDelivery(d.ID)
	if got.Status != models.DeliveryStatusPermanentFail {
		t.Errorf("Status: got %q, want permanent_fail", got.Status)
	}
	if got.Error != "character limit exceeded" {
		t.Errorf("Error: got %q", got.Error)
	}
}

func TestListRetryableDeliveries(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-retry")
	db.CreateJob(j)

	// d1: pending, no next_retry_at — should be returned
	d1 := makeDelivery("d-retry-1", j.ID)
	db.CreateDelivery(d1)

	// d2: pending, next_retry_at in future — should NOT be returned
	d2 := makeDelivery("d-retry-2", j.ID)
	db.CreateDelivery(d2)
	future := time.Now().Add(1 * time.Hour)
	db.UpdateDeliveryRetry(d2.ID, "err", 1, future)

	// d3: delivered — should NOT be returned
	d3 := makeDelivery("d-retry-3", j.ID)
	db.CreateDelivery(d3)
	db.UpdateDeliverySuccess(d3.ID, "https://example.com")

	deliveries, err := db.ListRetryableDeliveries()
	if err != nil {
		t.Fatalf("ListRetryableDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Errorf("expected 1 retryable delivery, got %d", len(deliveries))
	}
	if deliveries[0].ID != d1.ID {
		t.Errorf("expected delivery %q, got %q", d1.ID, deliveries[0].ID)
	}
}

func TestResetDeliveriesForRetry(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-manretry")
	db.CreateJob(j)

	d := makeDelivery("del-man", j.ID)
	db.CreateDelivery(d)
	db.UpdateDeliveryPermanentFail(d.ID, "too long", 10)

	if err := db.ResetDeliveriesForRetry(j.ID); err != nil {
		t.Fatalf("ResetDeliveriesForRetry: %v", err)
	}
	got, _ := db.GetDelivery(d.ID)
	if got.Status != models.DeliveryStatusPending {
		t.Errorf("Status: got %q, want pending", got.Status)
	}
	if got.AttemptCount != 0 {
		t.Errorf("AttemptCount: got %d, want 0", got.AttemptCount)
	}
}

// helper to verify JSON round-trip of media_ids
func TestJobMediaIDsRoundtrip(t *testing.T) {
	db := newTestDB(t)
	j := makeJob("job-media")
	j.MediaIDs = []string{"m1", "m2", "m3"}
	db.CreateJob(j)

	got, _ := db.GetJob(j.ID)
	if len(got.MediaIDs) != 3 {
		t.Errorf("expected 3 media IDs, got %d: %v", len(got.MediaIDs), got.MediaIDs)
	}

	// Confirm JSON serialisation produces expected shape
	b, _ := json.Marshal(got)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	ids, ok := m["media_ids"].([]interface{})
	if !ok || len(ids) != 3 {
		t.Errorf("JSON media_ids: got %v", m["media_ids"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/db/ -run "TestCreate|TestGet|TestList|TestUpdate|TestReset|TestJob" -v 2>&1 | head -40`
Expected: compile error — `db.CreateJob` undefined

- [ ] **Step 3: Implement jobs.go**

Create `backend/internal/db/jobs.go`:

```go
// backend/internal/db/jobs.go
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/sathyabhat/echobridge/internal/models"
)

func (db *DB) CreateJob(j *models.PostJob) error {
	mediaJSON, err := json.Marshal(j.MediaIDs)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO post_jobs (id, content, media_ids, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, j.ID, j.Content, string(mediaJSON), j.Status, j.CreatedAt, j.UpdatedAt)
	return err
}

func (db *DB) GetJob(id string) (*models.PostJob, error) {
	row := db.QueryRow(`SELECT id, content, media_ids, status, created_at, updated_at FROM post_jobs WHERE id = ?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return j, err
}

func (db *DB) ListJobs() ([]models.PostJob, error) {
	rows, err := db.Query(`SELECT id, content, media_ids, status, created_at, updated_at FROM post_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.PostJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func (db *DB) UpdateJobStatus(id string, status models.JobStatus) error {
	_, err := db.Exec(`UPDATE post_jobs SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now(), id)
	return err
}

func (db *DB) CreateDelivery(d *models.PostJobDelivery) error {
	_, err := db.Exec(`
		INSERT INTO post_job_deliveries
			(id, job_id, account_id, provider, display_name, status, attempt_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.JobID, d.AccountID, d.Provider, d.DisplayName, d.Status, d.AttemptCount, d.CreatedAt, d.UpdatedAt)
	return err
}

func (db *DB) GetDelivery(id string) (*models.PostJobDelivery, error) {
	row := db.QueryRow(`
		SELECT id, job_id, account_id, provider, display_name, status, post_url, error, attempt_count, next_retry_at, created_at, updated_at
		FROM post_job_deliveries WHERE id = ?
	`, id)
	d, err := scanDelivery(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

func (db *DB) ListDeliveriesByJob(jobID string) ([]models.PostJobDelivery, error) {
	rows, err := db.Query(`
		SELECT id, job_id, account_id, provider, display_name, status, post_url, error, attempt_count, next_retry_at, created_at, updated_at
		FROM post_job_deliveries WHERE job_id = ? ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []models.PostJobDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

func (db *DB) UpdateDeliverySuccess(id, postURL string) error {
	_, err := db.Exec(`
		UPDATE post_job_deliveries
		SET status = ?, post_url = ?, updated_at = ?
		WHERE id = ?
	`, models.DeliveryStatusDelivered, postURL, time.Now(), id)
	return err
}

func (db *DB) UpdateDeliveryRetry(id, errMsg string, attemptCount int, nextRetryAt time.Time) error {
	_, err := db.Exec(`
		UPDATE post_job_deliveries
		SET status = ?, error = ?, attempt_count = ?, next_retry_at = ?, updated_at = ?
		WHERE id = ?
	`, models.DeliveryStatusPending, errMsg, attemptCount, nextRetryAt, time.Now(), id)
	return err
}

func (db *DB) UpdateDeliveryPermanentFail(id, errMsg string, attemptCount int) error {
	_, err := db.Exec(`
		UPDATE post_job_deliveries
		SET status = ?, error = ?, attempt_count = ?, next_retry_at = NULL, updated_at = ?
		WHERE id = ?
	`, models.DeliveryStatusPermanentFail, errMsg, attemptCount, time.Now(), id)
	return err
}

func (db *DB) UpdateDeliveryProcessing(id string) error {
	_, err := db.Exec(`
		UPDATE post_job_deliveries SET status = ?, updated_at = ? WHERE id = ?
	`, models.DeliveryStatusProcessing, time.Now(), id)
	return err
}

// ListRetryableDeliveries returns pending deliveries whose next_retry_at is in
// the past or NULL (ready to be processed now).
func (db *DB) ListRetryableDeliveries() ([]models.PostJobDelivery, error) {
	rows, err := db.Query(`
		SELECT id, job_id, account_id, provider, display_name, status, post_url, error, attempt_count, next_retry_at, created_at, updated_at
		FROM post_job_deliveries
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []models.PostJobDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

// ResetDeliveriesForRetry resets all permanent_fail deliveries for a job back
// to pending with attempt_count = 0 so they are re-processed.
func (db *DB) ResetDeliveriesForRetry(jobID string) error {
	_, err := db.Exec(`
		UPDATE post_job_deliveries
		SET status = 'pending', attempt_count = 0, next_retry_at = NULL, error = NULL, updated_at = ?
		WHERE job_id = ? AND status = 'permanent_fail'
	`, time.Now(), jobID)
	return err
}

// ListPendingJobIDs returns job IDs for jobs that have at least one pending or
// processing delivery — used on startup to recover in-flight jobs.
func (db *DB) ListPendingJobIDs() ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT job_id FROM post_job_deliveries
		WHERE status IN ('pending', 'processing')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeriveJobStatus computes the aggregate status for a job from its deliveries.
func DeriveJobStatus(deliveries []models.PostJobDelivery) models.JobStatus {
	if len(deliveries) == 0 {
		return models.JobStatusPending
	}
	delivered, permFail, pending := 0, 0, 0
	for _, d := range deliveries {
		switch d.Status {
		case models.DeliveryStatusDelivered:
			delivered++
		case models.DeliveryStatusPermanentFail:
			permFail++
		default:
			pending++
		}
	}
	if pending > 0 {
		if delivered > 0 || permFail > 0 {
			return models.JobStatusProcessing
		}
		return models.JobStatusPending
	}
	if delivered == 0 {
		return models.JobStatusFailed
	}
	if permFail > 0 {
		return models.JobStatusPartial
	}
	return models.JobStatusCompleted
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(s scanner) (*models.PostJob, error) {
	var j models.PostJob
	var mediaJSON string
	if err := s.Scan(&j.ID, &j.Content, &mediaJSON, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, err
	}
	if mediaJSON != "" && mediaJSON != "null" {
		if err := json.Unmarshal([]byte(mediaJSON), &j.MediaIDs); err != nil {
			return nil, err
		}
	}
	return &j, nil
}

func scanDelivery(s scanner) (*models.PostJobDelivery, error) {
	var d models.PostJobDelivery
	var postURL, errMsg sql.NullString
	var nextRetryAt sql.NullTime
	if err := s.Scan(
		&d.ID, &d.JobID, &d.AccountID, &d.Provider, &d.DisplayName,
		&d.Status, &postURL, &errMsg, &d.AttemptCount, &nextRetryAt,
		&d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return nil, err
	}
	d.PostURL = postURL.String
	d.Error = errMsg.String
	if nextRetryAt.Valid {
		d.NextRetryAt = &nextRetryAt.Time
	}
	return &d, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/db/ -v 2>&1 | tail -20`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/db/jobs.go backend/internal/db/jobs_test.go
git commit -m "feat: implement DB layer for post_jobs and post_job_deliveries"
```

---

## Task 4: Implement the SSE broker

**Files:**
- Create: `backend/internal/queue/broker.go`
- Create: `backend/internal/queue/broker_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/queue/broker_test.go`:

```go
// backend/internal/queue/broker_test.go
package queue

import (
	"testing"
	"time"
)

func TestBroker_PublishReachesGlobalSubscriber(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("*")
	defer unsub()

	evt := Event{JobID: "job-1", DeliveryID: "del-1", Status: "delivered"}
	b.Publish(evt)

	select {
	case got := <-ch:
		if got.JobID != evt.JobID {
			t.Errorf("JobID: got %q, want %q", got.JobID, evt.JobID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for event")
	}
}

func TestBroker_PublishReachesJobSubscriber(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("job-42")
	defer unsub()

	// Event for a different job should NOT arrive
	b.Publish(Event{JobID: "job-99", DeliveryID: "d1", Status: "delivered"})
	// Event for our job should arrive
	b.Publish(Event{JobID: "job-42", DeliveryID: "d2", Status: "delivered"})

	select {
	case got := <-ch:
		if got.JobID != "job-42" {
			t.Errorf("expected job-42, got %q", got.JobID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for job-specific event")
	}
}

func TestBroker_UnsubscribeRemovesChannel(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("job-1")
	unsub()

	// After unsubscribing, publishing should not block and channel should be empty
	b.Publish(Event{JobID: "job-1", DeliveryID: "d1", Status: "delivered"})

	select {
	case <-ch:
		t.Error("received event on unsubscribed channel")
	case <-time.After(50 * time.Millisecond):
		// correct: nothing received
	}
}

func TestBroker_SlowSubscriberDoesNotBlock(t *testing.T) {
	b := NewBroker()
	// Subscribe with a full channel (capacity 1, never drained)
	_, _ = b.Subscribe("job-1")

	// Publish 10 events — none should block
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			b.Publish(Event{JobID: "job-1", DeliveryID: "d", Status: "delivered"})
		}
		close(done)
	}()

	select {
	case <-done:
		// correct
	case <-time.After(500 * time.Millisecond):
		t.Error("Publish blocked on slow subscriber")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/queue/ -v 2>&1 | head -20`
Expected: compile error — package `queue` does not exist

- [ ] **Step 3: Implement broker.go**

Create `backend/internal/queue/broker.go`:

```go
// backend/internal/queue/broker.go
package queue

import "sync"

// Event is published to SSE subscribers after each delivery state change.
type Event struct {
	JobID        string `json:"job_id"`
	DeliveryID   string `json:"delivery_id"`
	Provider     string `json:"provider"`
	DisplayName  string `json:"display_name"`
	Status       string `json:"status"`
	PostURL      string `json:"post_url,omitempty"`
	Error        string `json:"error,omitempty"`
	AttemptCount int    `json:"attempt_count"`
}

// Broker is a lightweight pub/sub hub for SSE events.
// Key "*" is used for global (all-jobs) subscribers.
type Broker struct {
	mu   sync.Mutex
	subs map[string][]chan Event
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[string][]chan Event)}
}

// Subscribe registers a channel for events with the given jobID.
// Pass "*" to receive all events. The returned function removes the subscription.
func (b *Broker) Subscribe(jobID string) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[jobID] = append(b.subs[jobID], ch)
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.subs[jobID]
		for i, c := range list {
			if c == ch {
				b.subs[jobID] = append(list[:i], list[i+1:]...)
				break
			}
		}
	}
}

// Publish sends an event to global subscribers and to subscribers for the event's jobID.
// Non-blocking: drops the event if a subscriber's channel is full.
func (b *Broker) Publish(evt Event) {
	b.mu.Lock()
	targets := make([]chan Event, 0)
	for _, ch := range b.subs["*"] {
		targets = append(targets, ch)
	}
	if evt.JobID != "*" {
		for _, ch := range b.subs[evt.JobID] {
			targets = append(targets, ch)
		}
	}
	b.mu.Unlock()

	for _, ch := range targets {
		select {
		case ch <- evt:
		default:
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/queue/ -run TestBroker -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/queue/broker.go backend/internal/queue/broker_test.go
git commit -m "feat: implement SSE broker for queue events"
```

---

## Task 5: Implement the queue worker

**Files:**
- Create: `backend/internal/queue/queue.go`
- Create: `backend/internal/queue/queue_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/queue/queue_test.go`:

```go
// backend/internal/queue/queue_test.go
package queue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sathyabhat/echobridge/internal/db"
	"github.com/sathyabhat/echobridge/internal/models"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func makeTestJob(t *testing.T, database *db.DB, accountID string) *models.PostJob {
	t.Helper()
	j := &models.PostJob{
		ID:        uuid.New().String(),
		Content:   "test post",
		Status:    models.JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateJob(j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	d := &models.PostJobDelivery{
		ID:          uuid.New().String(),
		JobID:       j.ID,
		AccountID:   accountID,
		Provider:    "mastodon",
		DisplayName: "alice@mastodon.social",
		Status:      models.DeliveryStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := database.CreateDelivery(d); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	return j
}

// stubProvider is a minimal Provider for queue tests.
type stubProvider struct {
	postFunc func() (*models.PostResult, error)
}

func (s *stubProvider) Name() string { return "mastodon" }
func (s *stubProvider) Post(_ context.Context, _ *models.Account, _ string, _ []string) (*models.PostResult, error) {
	return s.postFunc()
}
func (s *stubProvider) UploadMedia(_ context.Context, _ *models.Account, _ interface{}, _, _ string) (string, error) {
	return "", nil
}

func TestQueue_SuccessfulDelivery(t *testing.T) {
	database := newTestDB(t)
	broker := NewBroker()

	accountID := uuid.New().String()
	account := &models.Account{
		ID: accountID, Provider: models.ProviderMastodon,
		DisplayName: "alice", AccessToken: "tok",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	database.CreateAccount(account)

	provider := &stubProvider{postFunc: func() (*models.PostResult, error) {
		return &models.PostResult{Success: true, PostURL: "https://mastodon.social/@alice/1"}, nil
	}}

	q := New(database, broker, map[string]providerFactory{
		"mastodon": func(_ *models.Account) (workerProvider, error) { return provider, nil },
	}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	q.Start(ctx)

	job := makeTestJob(t, database, accountID)
	q.Enqueue(job.ID)

	// Wait for delivery to be processed
	time.Sleep(300 * time.Millisecond)

	deliveries, _ := database.ListDeliveriesByJob(job.ID)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].Status != models.DeliveryStatusDelivered {
		t.Errorf("Status: got %q, want delivered", deliveries[0].Status)
	}
	if deliveries[0].PostURL != "https://mastodon.social/@alice/1" {
		t.Errorf("PostURL: got %q", deliveries[0].PostURL)
	}

	j, _ := database.GetJob(job.ID)
	if j.Status != models.JobStatusCompleted {
		t.Errorf("Job status: got %q, want completed", j.Status)
	}
}

func TestQueue_TransientFailureSchedulesRetry(t *testing.T) {
	database := newTestDB(t)
	broker := NewBroker()

	accountID := uuid.New().String()
	account := &models.Account{
		ID: accountID, Provider: models.ProviderMastodon,
		DisplayName: "alice", AccessToken: "tok",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	database.CreateAccount(account)

	provider := &stubProvider{postFunc: func() (*models.PostResult, error) {
		return nil, &ProviderError{StatusCode: 503, Message: "service unavailable"}
	}}

	q := New(database, broker, map[string]providerFactory{
		"mastodon": func(_ *models.Account) (workerProvider, error) { return provider, nil },
	}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	q.Start(ctx)

	job := makeTestJob(t, database, accountID)
	q.Enqueue(job.ID)
	time.Sleep(300 * time.Millisecond)

	deliveries, _ := database.ListDeliveriesByJob(job.ID)
	if deliveries[0].Status != models.DeliveryStatusPending {
		t.Errorf("Status: got %q, want pending (scheduled for retry)", deliveries[0].Status)
	}
	if deliveries[0].AttemptCount != 1 {
		t.Errorf("AttemptCount: got %d, want 1", deliveries[0].AttemptCount)
	}
	if deliveries[0].NextRetryAt == nil {
		t.Error("NextRetryAt should be set after transient failure")
	}
}

func TestQueue_PermanentFailure(t *testing.T) {
	database := newTestDB(t)
	broker := NewBroker()

	accountID := uuid.New().String()
	account := &models.Account{
		ID: accountID, Provider: models.ProviderMastodon,
		DisplayName: "alice", AccessToken: "tok",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	database.CreateAccount(account)

	provider := &stubProvider{postFunc: func() (*models.PostResult, error) {
		return nil, &ProviderError{StatusCode: 422, Message: "character limit exceeded"}
	}}

	q := New(database, broker, map[string]providerFactory{
		"mastodon": func(_ *models.Account) (workerProvider, error) { return provider, nil },
	}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	q.Start(ctx)

	job := makeTestJob(t, database, accountID)
	q.Enqueue(job.ID)
	time.Sleep(300 * time.Millisecond)

	deliveries, _ := database.ListDeliveriesByJob(job.ID)
	if deliveries[0].Status != models.DeliveryStatusPermanentFail {
		t.Errorf("Status: got %q, want permanent_fail", deliveries[0].Status)
	}

	j, _ := database.GetJob(job.ID)
	if j.Status != models.JobStatusFailed {
		t.Errorf("Job status: got %q, want failed", j.Status)
	}
}

func TestQueue_MaxAttemptsBecomePermanentFail(t *testing.T) {
	database := newTestDB(t)
	broker := NewBroker()

	accountID := uuid.New().String()
	account := &models.Account{
		ID: accountID, Provider: models.ProviderMastodon,
		DisplayName: "alice", AccessToken: "tok",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	database.CreateAccount(account)

	// Delivery already has 9 attempts (one more transient → permanent_fail)
	provider := &stubProvider{postFunc: func() (*models.PostResult, error) {
		return nil, &ProviderError{StatusCode: 503, Message: "still down"}
	}}

	q := New(database, broker, map[string]providerFactory{
		"mastodon": func(_ *models.Account) (workerProvider, error) { return provider, nil },
	}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	q.Start(ctx)

	job := makeTestJob(t, database, accountID)
	// Manually set attempt_count to 9 directly in DB
	database.UpdateDeliveryRetry(
		func() string {
			dels, _ := database.ListDeliveriesByJob(job.ID)
			return dels[0].ID
		}(),
		"prior error", 9, time.Now().Add(-1*time.Second),
	)

	q.Enqueue(job.ID)
	time.Sleep(300 * time.Millisecond)

	deliveries, _ := database.ListDeliveriesByJob(job.ID)
	if deliveries[0].Status != models.DeliveryStatusPermanentFail {
		t.Errorf("Status: got %q, want permanent_fail after 10 attempts", deliveries[0].Status)
	}
}

func TestQueue_CrashRecoveryOnStart(t *testing.T) {
	database := newTestDB(t)
	broker := NewBroker()

	accountID := uuid.New().String()
	account := &models.Account{
		ID: accountID, Provider: models.ProviderMastodon,
		DisplayName: "alice", AccessToken: "tok",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	database.CreateAccount(account)

	called := make(chan struct{}, 1)
	provider := &stubProvider{postFunc: func() (*models.PostResult, error) {
		called <- struct{}{}
		return &models.PostResult{Success: true, PostURL: "https://mastodon.social/@alice/1"}, nil
	}}

	// Create a job that is "pending" as if the server crashed mid-flight
	job := makeTestJob(t, database, accountID)
	_ = job

	q := New(database, broker, map[string]providerFactory{
		"mastodon": func(_ *models.Account) (workerProvider, error) { return provider, nil },
	}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Start should automatically pick up the pending job
	q.Start(ctx)

	select {
	case <-called:
		// correct: job was re-queued and processed
	case <-time.After(1 * time.Second):
		t.Error("crash recovery: pending job was not re-queued on start")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/queue/ -v 2>&1 | head -20`
Expected: compile error — `queue.New`, `ProviderError` not defined

- [ ] **Step 3: Implement queue.go**

Create `backend/internal/queue/queue.go`:

```go
// backend/internal/queue/queue.go
package queue

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sathyabhat/echobridge/internal/db"
	"github.com/sathyabhat/echobridge/internal/models"
)

const maxAttempts = 10
const maxBackoff = 2 * time.Hour

// ProviderError carries an HTTP status code so the worker can classify
// transient vs permanent failures.
type ProviderError struct {
	StatusCode int
	Message    string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// isPermanentError returns true for 4xx errors that should not be retried.
func isPermanentError(err error) bool {
	var pe *ProviderError
	if !isProviderError(err, &pe) {
		return false
	}
	switch pe.StatusCode {
	case 400, 401, 403, 413, 422:
		return true
	}
	return false
}

func isProviderError(err error, out **ProviderError) bool {
	if pe, ok := err.(*ProviderError); ok {
		*out = pe
		return true
	}
	return false
}

func backoffDuration(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// workerProvider is the subset of providers.Provider used by the queue worker.
// Defined as an interface so tests can inject stubs.
type workerProvider interface {
	Name() string
	Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
}

// providerFactory creates a workerProvider for a given account.
type providerFactory func(account *models.Account) (workerProvider, error)

// Queue manages async post delivery.
type Queue struct {
	db        *db.DB
	broker    *Broker
	factories map[string]providerFactory
	jobs      chan string
	workers   int
}

// New creates a Queue. factories maps provider names to factory functions.
// workers is the number of concurrent worker goroutines.
func New(database *db.DB, broker *Broker, factories map[string]providerFactory, workers int) *Queue {
	return &Queue{
		db:        database,
		broker:    broker,
		factories: factories,
		jobs:      make(chan string, 256),
		workers:   workers,
	}
}

// Enqueue adds a job ID to the processing channel.
func (q *Queue) Enqueue(jobID string) {
	select {
	case q.jobs <- jobID:
	default:
		log.Printf("queue: channel full, dropping job %s", jobID)
	}
}

// Start launches worker goroutines and the retry scheduler.
// It blocks until ctx is cancelled.
func (q *Queue) Start(ctx context.Context) {
	// Crash recovery: re-enqueue jobs that were pending when server last stopped.
	ids, err := q.db.ListPendingJobIDs()
	if err != nil {
		log.Printf("queue: crash recovery query failed: %v", err)
	}
	for _, id := range ids {
		q.Enqueue(id)
	}

	for i := 0; i < q.workers; i++ {
		go q.runWorker(ctx)
	}
	go q.runRetryScheduler(ctx)
}

func (q *Queue) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-q.jobs:
			q.processJob(ctx, jobID)
		}
	}
}

func (q *Queue) processJob(ctx context.Context, jobID string) {
	if err := q.db.UpdateJobStatus(jobID, models.JobStatusProcessing); err != nil {
		log.Printf("queue: failed to mark job %s processing: %v", jobID, err)
		return
	}

	deliveries, err := q.db.ListDeliveriesByJob(jobID)
	if err != nil {
		log.Printf("queue: failed to list deliveries for job %s: %v", jobID, err)
		return
	}

	job, err := q.db.GetJob(jobID)
	if err != nil || job == nil {
		log.Printf("queue: job %s not found: %v", jobID, err)
		return
	}

	now := time.Now()
	for i := range deliveries {
		d := &deliveries[i]
		// Skip already-delivered or not-yet-due deliveries
		if d.Status == models.DeliveryStatusDelivered || d.Status == models.DeliveryStatusPermanentFail {
			continue
		}
		if d.NextRetryAt != nil && d.NextRetryAt.After(now) {
			continue
		}

		q.db.UpdateDeliveryProcessing(d.ID)
		q.processDelivery(ctx, job, d)
	}

	// Recompute and save overall job status
	updatedDeliveries, _ := q.db.ListDeliveriesByJob(jobID)
	newStatus := db.DeriveJobStatus(updatedDeliveries)
	q.db.UpdateJobStatus(jobID, newStatus)
}

func (q *Queue) processDelivery(ctx context.Context, job *models.PostJob, d *models.PostJobDelivery) {
	account, err := q.db.GetAccount(d.AccountID)
	if err != nil || account == nil {
		q.db.UpdateDeliveryPermanentFail(d.ID, "account not found", d.AttemptCount+1)
		q.publishDeliveryEvent(d.ID, d.JobID, d.Provider, d.DisplayName, string(models.DeliveryStatusPermanentFail), "", "account not found", d.AttemptCount+1)
		return
	}

	factory, ok := q.factories[string(account.Provider)]
	if !ok {
		q.db.UpdateDeliveryPermanentFail(d.ID, fmt.Sprintf("unsupported provider: %s", account.Provider), d.AttemptCount+1)
		q.publishDeliveryEvent(d.ID, d.JobID, d.Provider, d.DisplayName, string(models.DeliveryStatusPermanentFail), "", "unsupported provider", d.AttemptCount+1)
		return
	}

	provider, err := factory(account)
	if err != nil {
		q.db.UpdateDeliveryPermanentFail(d.ID, err.Error(), d.AttemptCount+1)
		q.publishDeliveryEvent(d.ID, d.JobID, d.Provider, d.DisplayName, string(models.DeliveryStatusPermanentFail), "", err.Error(), d.AttemptCount+1)
		return
	}

	// Upload media for this delivery (reuse local files)
	var mediaIDs []string
	for _, localMediaID := range job.MediaIDs {
		media, err := q.db.GetMedia(localMediaID)
		if err != nil || media == nil {
			continue
		}
		// workerProvider does not include UploadMedia; media upload is handled
		// by the concrete provider via the full providers.Provider interface.
		// We only have workerProvider here, so skip media upload in queue.
		// The full provider handles UploadMedia when called via handleCreatePost
		// for synchronous uploads. Queue workers re-use already-uploaded media
		// by passing local IDs; provider implementations may need adapting.
		// For now, attach local IDs and let the provider handle them.
		mediaIDs = append(mediaIDs, media.ID)
	}

	result, err := provider.Post(ctx, account, job.Content, mediaIDs)
	newAttempt := d.AttemptCount + 1

	if err != nil {
		if isPermanentError(err) || newAttempt >= maxAttempts {
			q.db.UpdateDeliveryPermanentFail(d.ID, err.Error(), newAttempt)
			q.publishDeliveryEvent(d.ID, d.JobID, d.Provider, d.DisplayName, string(models.DeliveryStatusPermanentFail), "", err.Error(), newAttempt)
		} else {
			nextRetry := time.Now().Add(backoffDuration(newAttempt))
			q.db.UpdateDeliveryRetry(d.ID, err.Error(), newAttempt, nextRetry)
			q.publishDeliveryEvent(d.ID, d.JobID, d.Provider, d.DisplayName, string(models.DeliveryStatusPending), "", err.Error(), newAttempt)
		}
		return
	}

	postURL := ""
	if result != nil {
		postURL = result.PostURL
	}
	q.db.UpdateDeliverySuccess(d.ID, postURL)
	q.publishDeliveryEvent(d.ID, d.JobID, d.Provider, d.DisplayName, string(models.DeliveryStatusDelivered), postURL, "", newAttempt)
}

func (q *Queue) publishDeliveryEvent(deliveryID, jobID, provider, displayName, status, postURL, errMsg string, attemptCount int) {
	q.broker.Publish(Event{
		JobID:        jobID,
		DeliveryID:   deliveryID,
		Provider:     provider,
		DisplayName:  displayName,
		Status:       status,
		PostURL:      postURL,
		Error:        errMsg,
		AttemptCount: attemptCount,
	})
}

func (q *Queue) runRetryScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.scheduleRetries()
		}
	}
}

func (q *Queue) scheduleRetries() {
	deliveries, err := q.db.ListRetryableDeliveries()
	if err != nil {
		log.Printf("queue: retry scheduler query failed: %v", err)
		return
	}
	seen := make(map[string]bool)
	for _, d := range deliveries {
		if !seen[d.JobID] {
			seen[d.JobID] = true
			q.Enqueue(d.JobID)
		}
	}
}

// WorkerCount returns the configured number of workers (used by main to read
// ECHOBRIDGE_WORKERS env var).
func WorkerCount() int {
	if v := os.Getenv("ECHOBRIDGE_WORKERS"); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return 3
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./internal/queue/ -v -timeout 30s`
Expected: all PASS

- [ ] **Step 5: Run all tests**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./... -timeout 30s`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/queue/queue.go backend/internal/queue/queue_test.go
git commit -m "feat: implement queue worker pool with exponential backoff retry"
```

---

## Task 6: Wire the queue into the server

**Files:**
- Modify: `backend/internal/api/router.go`
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Add Queue and Broker fields to Server**

In `backend/internal/api/router.go`, update the `Server` struct and `NewServer` signature:

```go
// Add imports at top of file
import (
	// ... existing imports ...
	"github.com/sathyabhat/echobridge/internal/queue"
)

type Server struct {
	db         *db.DB
	router     *chi.Mux
	uploadDir  string
	baseURL    string
	pathPrefix string
	mastodon   MastodonService
	bluesky    BlueskyService
	queue      *queue.Queue
	broker     *queue.Broker
}

func NewServer(database *db.DB, uploadDir, baseURL, pathPrefix string, q *queue.Queue, broker *queue.Broker) *Server {
	s := &Server{
		db:         database,
		router:     chi.NewRouter(),
		uploadDir:  uploadDir,
		baseURL:    baseURL,
		pathPrefix: pathPrefix,
		mastodon:   providers.NewMastodon(),
		bluesky:    providers.NewBluesky(),
		queue:      q,
		broker:     broker,
	}
	s.setupRoutes()
	return s
}
```

- [ ] **Step 2: Register new routes in setupRoutes**

In `backend/internal/api/router.go`, inside the `r.Route(s.pathPrefix+"/api", ...)` block, replace:

```go
		r.Post("/posts", s.handleCreatePost)
```

with:

```go
		r.Get("/posts", s.handleListJobs)
		r.Post("/posts", s.handleCreatePost)
		r.Get("/posts/{jobID}", s.handleGetJob)
		r.Post("/posts/{jobID}/retry", s.handleRetryJob)

		r.Get("/events", s.handleGlobalSSE)
		r.Get("/events/{jobID}", s.handleJobSSE)
```

- [ ] **Step 3: Update main.go to instantiate Queue and Broker**

Replace the server creation block in `backend/cmd/server/main.go`:

```go
import (
	// ... existing imports ...
	"github.com/sathyabhat/echobridge/internal/providers"
	"github.com/sathyabhat/echobridge/internal/queue"
)

// Inside main(), after database is created and before server:

broker := queue.NewBroker()

mastodonProvider := providers.NewMastodon()
blueskyProvider := providers.NewBluesky()

factories := map[string]queue.ProviderFactory{
	"mastodon": func(account *models.Account) (queue.WorkerProvider, error) {
		return mastodonProvider, nil
	},
	"bluesky": func(account *models.Account) (queue.WorkerProvider, error) {
		return blueskyProvider, nil
	},
}

q := queue.New(database, broker, factories, queue.WorkerCount())

server := api.NewServer(database, uploadDir, baseURL, pathPrefix, q, broker)
```

- [ ] **Step 4: Start the queue in main.go**

After `go server.StartTokenRefresher(ctx, 60*time.Minute)`, add:

```go
	q.Start(ctx)
```

- [ ] **Step 5: Export types needed by main.go**

In `backend/internal/queue/queue.go`, export the factory types so main.go can use them:

```go
// WorkerProvider is the exported alias used by main.go to build factory maps.
type WorkerProvider = workerProvider

// ProviderFactory is the exported alias used by main.go.
type ProviderFactory = providerFactory
```

- [ ] **Step 6: Verify it builds**

Run: `cd /home/sathyabhat/code/echobridge/backend && go build ./...`
Expected: no errors

- [ ] **Step 7: Run all tests**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./... -timeout 30s`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add backend/internal/api/router.go backend/cmd/server/main.go backend/internal/queue/queue.go
git commit -m "feat: wire queue and broker into server and register new routes"
```

---

## Task 7: Implement new API handlers

**Files:**
- Modify: `backend/internal/api/handlers.go`

- [ ] **Step 1: Rewrite handleCreatePost to be async**

Replace the `handleCreatePost` function in `backend/internal/api/handlers.go`:

```go
func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	var req models.PostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "Content is required")
		return
	}
	if len(req.AccountIDs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one account is required")
		return
	}

	jobID := uuid.New().String()
	now := time.Now()
	job := &models.PostJob{
		ID:        jobID,
		Content:   req.Content,
		MediaIDs:  req.MediaIDs,
		Status:    models.JobStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.db.CreateJob(job); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create job")
		return
	}

	for _, accountID := range req.AccountIDs {
		account, err := s.db.GetAccount(accountID)
		displayName := accountID
		provider := "unknown"
		if err == nil && account != nil {
			displayName = account.DisplayName
			provider = string(account.Provider)
		}
		d := &models.PostJobDelivery{
			ID:          uuid.New().String(),
			JobID:       jobID,
			AccountID:   accountID,
			Provider:    provider,
			DisplayName: displayName,
			Status:      models.DeliveryStatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.db.CreateDelivery(d); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to create delivery")
			return
		}
	}

	s.queue.Enqueue(jobID)
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "pending"})
}
```

- [ ] **Step 2: Add handleListJobs**

Add after handleCreatePost:

```go
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.db.ListJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list jobs")
		return
	}
	if jobs == nil {
		jobs = []models.PostJob{}
	}
	for i := range jobs {
		deliveries, err := s.db.ListDeliveriesByJob(jobs[i].ID)
		if err == nil {
			jobs[i].Deliveries = deliveries
		}
	}
	writeJSON(w, http.StatusOK, jobs)
}
```

- [ ] **Step 3: Add handleGetJob**

```go
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.db.GetJob(jobID)
	if err != nil || job == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}
	deliveries, err := s.db.ListDeliveriesByJob(jobID)
	if err == nil {
		job.Deliveries = deliveries
	}
	writeJSON(w, http.StatusOK, job)
}
```

- [ ] **Step 4: Add handleRetryJob**

```go
func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.db.GetJob(jobID)
	if err != nil || job == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}
	if err := s.db.ResetDeliveriesForRetry(jobID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reset deliveries")
		return
	}
	s.db.UpdateJobStatus(jobID, models.JobStatusPending)
	s.queue.Enqueue(jobID)

	job, _ = s.db.GetJob(jobID)
	deliveries, _ := s.db.ListDeliveriesByJob(jobID)
	if job != nil {
		job.Deliveries = deliveries
	}
	writeJSON(w, http.StatusOK, job)
}
```

- [ ] **Step 5: Add handleGlobalSSE and handleJobSSE**

Add the SSE handlers. These require adding `"encoding/json"` (already imported) and `"fmt"` (already imported):

```go
func (s *Server) handleGlobalSSE(w http.ResponseWriter, r *http.Request) {
	s.serveSSE(w, r, "*")
}

func (s *Server) handleJobSSE(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	s.serveSSE(w, r, jobID)
}

func (s *Server) serveSSE(w http.ResponseWriter, r *http.Request, subscriptionKey string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := s.broker.Subscribe(subscriptionKey)
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 6: Add missing imports to handlers.go**

The handlers use `models.PostJob`, `models.PostJobDelivery`, `models.JobStatusPending`, `models.DeliveryStatusPending`, and `queue.Queue`. Add to the import block:

```go
	"github.com/sathyabhat/echobridge/internal/queue"
```

Wait — the handlers don't directly import `queue`; they use `s.queue` and `s.broker` which are already typed in `router.go`. No new import needed in `handlers.go`. Just verify the build compiles.

- [ ] **Step 7: Verify build**

Run: `cd /home/sathyabhat/code/echobridge/backend && go build ./...`
Expected: no errors

- [ ] **Step 8: Run all tests**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./... -timeout 30s`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add backend/internal/api/handlers.go
git commit -m "feat: async handleCreatePost; add list/get/retry/SSE job handlers"
```

---

## Task 8: Handle media in queue workers

**Files:**
- Modify: `backend/internal/queue/queue.go`
- Modify: `backend/internal/api/services.go` (add UploadMedia to workerProvider)

The queue worker needs to upload media per-delivery. The `workerProvider` interface must include `UploadMedia`. Update it:

- [ ] **Step 1: Extend workerProvider interface in queue.go**

In `backend/internal/queue/queue.go`, update the `workerProvider` interface:

```go
type workerProvider interface {
	Name() string
	Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error)
	UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error)
}
```

Add `"io"` to the import block in queue.go.

- [ ] **Step 2: Update processDelivery to upload media**

Replace the media section in `processDelivery` in `backend/internal/queue/queue.go`:

```go
	// Upload media to the platform for this delivery
	var mediaIDs []string
	for _, localMediaID := range job.MediaIDs {
		media, err := q.db.GetMedia(localMediaID)
		if err != nil || media == nil {
			continue
		}
		file, err := os.Open(media.Path)
		if err != nil {
			continue
		}
		remoteID, err := provider.UploadMedia(ctx, account, file, media.Filename, media.ContentType)
		file.Close()
		if err != nil {
			continue
		}
		mediaIDs = append(mediaIDs, remoteID)
	}
```

- [ ] **Step 3: Update stub in queue_test.go**

In `backend/internal/queue/queue_test.go`, update `stubProvider`:

```go
type stubProvider struct {
	postFunc func() (*models.PostResult, error)
}

func (s *stubProvider) Name() string { return "mastodon" }
func (s *stubProvider) Post(_ context.Context, _ *models.Account, _ string, _ []string) (*models.PostResult, error) {
	return s.postFunc()
}
func (s *stubProvider) UploadMedia(_ context.Context, _ *models.Account, _ io.Reader, _, _ string) (string, error) {
	return "", nil
}
```

Add `"io"` to imports in `queue_test.go`.

- [ ] **Step 4: Verify build and tests**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./... -timeout 30s`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/queue/queue.go backend/internal/queue/queue_test.go
git commit -m "feat: queue workers upload media per delivery before posting"
```

---

## Task 9: Handle Bluesky token refresh in queue

**Files:**
- Modify: `backend/internal/queue/queue.go`

The existing `handleCreatePost` refreshed Bluesky tokens before posting. Workers must do the same.

- [ ] **Step 1: Add token refresh interface to queue**

In `backend/internal/queue/queue.go`, add a `tokenRefresher` interface and a field on `Queue`:

```go
type tokenRefresher interface {
	RefreshSession(pdsURL, refreshJwt string) (*blueskySessionResponse, error)
}

type blueskySessionResponse struct {
	AccessJwt  string
	RefreshJwt string
}
```

Actually, rather than duplicating the providers type, use a simple function field on Queue:

```go
// RefreshBlueskyToken, if set, is called before posting to a Bluesky account.
// It should return new access and refresh tokens.
type TokenRefreshFunc func(account *models.Account) (accessToken, refreshToken string, err error)
```

Add to `Queue` struct:

```go
	refreshToken TokenRefreshFunc
```

Add to `New`:

```go
func New(database *db.DB, broker *Broker, factories map[string]providerFactory, workers int, refreshFn TokenRefreshFunc) *Queue {
	return &Queue{
		db:           database,
		broker:       broker,
		factories:    factories,
		jobs:         make(chan string, 256),
		workers:      workers,
		refreshToken: refreshFn,
	}
}
```

- [ ] **Step 2: Call refresh in processDelivery before posting**

In `processDelivery`, after the account is fetched and before the provider factory is called, add:

```go
	// Refresh Bluesky tokens before posting; they expire ~2 hours.
	if account.Provider == models.ProviderBluesky && q.refreshToken != nil {
		if newAccess, newRefresh, err := q.refreshToken(account); err == nil {
			q.db.UpdateAccountTokens(account.ID, newAccess, newRefresh)
			account.AccessToken = newAccess
			account.RefreshToken = newRefresh
		}
	}
```

- [ ] **Step 3: Update main.go to pass refresh function**

In `backend/cmd/server/main.go`, update the `queue.New` call:

```go
refreshFn := queue.TokenRefreshFunc(func(account *models.Account) (string, string, error) {
	session, err := blueskyProvider.RefreshSession(account.InstanceURL, account.RefreshToken)
	if err != nil {
		return "", "", err
	}
	return session.AccessJwt, session.RefreshJwt, nil
})

q := queue.New(database, broker, factories, queue.WorkerCount(), refreshFn)
```

- [ ] **Step 4: Update queue_test.go to pass nil for refreshFn**

In all `queue.New(...)` calls in `queue_test.go`, add a `nil` argument at the end:

```go
q := New(database, broker, map[string]providerFactory{...}, 1, nil)
```

- [ ] **Step 5: Verify build and tests**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./... -timeout 30s`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/queue/queue.go backend/internal/queue/queue_test.go backend/cmd/server/main.go
git commit -m "feat: refresh Bluesky tokens in queue workers before posting"
```

---

## Task 10: Update frontend compose page for async flow

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/js/app.js`
- Modify: `frontend/css/style.css`

- [ ] **Step 1: Add History nav link to index.html**

In `frontend/index.html`, replace the nav `<ul>`:

```html
            <ul>
                <li><a href="index.html" class="contrast">Compose</a></li>
                <li><a href="history.html">History</a></li>
                <li><a href="profile.html">Accounts</a></li>
            </ul>
```

- [ ] **Step 2: Update results section in index.html**

Replace the `results-section` article:

```html
        <article id="results-section" class="hidden">
            <header>
                <h3>Posting Status</h3>
            </header>
            <div id="results-list"></div>
            <button id="post-again-btn" class="hidden outline" type="button">Post Again</button>
        </article>
```

- [ ] **Step 3: Rewrite setupFormSubmit in app.js**

Replace the `setupFormSubmit` function and `displayResults` function in `frontend/js/app.js`:

```javascript
let currentSSE = null;

function setupFormSubmit() {
    const form = document.getElementById('compose-form');
    const submitBtn = document.getElementById('submit-btn');
    const postAgainBtn = document.getElementById('post-again-btn');

    postAgainBtn.addEventListener('click', () => {
        document.getElementById('results-section').classList.add('hidden');
        postAgainBtn.classList.add('hidden');
        document.getElementById('results-list').innerHTML = '';
        document.getElementById('content').value = '';
        updateCharCounter();
        mediaFiles = [];
        renderMediaPreviews();
        document.getElementById('media').value = '';
        if (currentSSE) { currentSSE.close(); currentSSE = null; }
    });

    form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const content = document.getElementById('content').value.trim();
        if (!content) { alert('Please enter a message'); return; }

        const checkedBoxes = document.querySelectorAll('input[name="account_ids"]:checked');
        const accountIds = Array.from(checkedBoxes).map(cb => cb.value);
        if (accountIds.length === 0) { alert('Please select at least one account'); return; }

        submitBtn.disabled = true;
        submitBtn.ariaBusy = 'true';
        submitBtn.textContent = 'Posting...';

        try {
            let uploadedMediaIds = [];
            for (const file of mediaFiles) {
                const formData = new FormData();
                formData.append('file', file);
                const uploadRes = await fetch(`${API_BASE}/media/upload`, { method: 'POST', body: formData });
                if (!uploadRes.ok) throw new Error(`Failed to upload ${file.name}`);
                const uploadData = await uploadRes.json();
                uploadedMediaIds.push(uploadData.id);
            }

            const response = await fetch(`${API_BASE}/posts`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content, media_ids: uploadedMediaIds, account_ids: accountIds })
            });

            if (!response.ok) {
                const err = await response.json();
                throw new Error(err.error || 'Failed to submit post');
            }

            const { job_id } = await response.json();

            // Show per-account spinners
            const section = document.getElementById('results-section');
            const list = document.getElementById('results-list');
            section.classList.remove('hidden');

            const accountMeta = {};
            checkedBoxes.forEach(cb => {
                accountMeta[cb.value] = { provider: cb.dataset.provider, label: cb.parentElement.textContent.trim() };
            });

            list.innerHTML = accountIds.map(id => {
                const meta = accountMeta[id] || { provider: 'unknown', label: id };
                return `<div class="result-item result-pending" id="delivery-${id}" data-account="${id}">
                    <span class="delivery-icon">⏳</span>
                    <strong>${meta.label}</strong>
                    <span class="delivery-status"> Queued...</span>
                </div>`;
            }).join('');

            // Subscribe to per-job SSE
            if (currentSSE) currentSSE.close();
            currentSSE = new EventSource(`${API_BASE}/events/${job_id}`);
            currentSSE.onmessage = (e) => {
                const evt = JSON.parse(e.data);
                updateDeliveryRow(evt, accountIds);
            };
            currentSSE.onerror = () => { currentSSE.close(); currentSSE = null; };

        } catch (error) {
            alert('Error posting: ' + error.message);
        } finally {
            submitBtn.disabled = false;
            submitBtn.ariaBusy = 'false';
            submitBtn.textContent = 'Post';
        }
    });
}

function updateDeliveryRow(evt, accountIds) {
    // Find delivery row by account_id matching evt fields
    // We match by job delivery broadcast: look for a row with matching provider+display_name
    const rows = document.querySelectorAll('.result-item[data-account]');
    let target = null;
    rows.forEach(row => {
        const label = row.querySelector('strong').textContent;
        if (label.includes(evt.display_name) || label.includes(evt.provider)) {
            target = row;
        }
    });
    if (!target) return;

    const icon = target.querySelector('.delivery-icon');
    const status = target.querySelector('.delivery-status');

    switch (evt.status) {
        case 'delivered':
            target.className = 'result-item result-success';
            icon.textContent = '✅';
            status.innerHTML = evt.post_url
                ? ` <a href="${evt.post_url}" target="_blank">View post</a>`
                : ' Posted!';
            break;
        case 'permanent_fail':
            target.className = 'result-item result-error';
            icon.textContent = '❌';
            status.textContent = ` Failed: ${evt.error}`;
            break;
        default: // pending / retrying
            icon.textContent = '🔄';
            status.textContent = ` Retrying (attempt ${evt.attempt_count})...`;
    }

    // Check if all settled
    const pending = document.querySelectorAll('.result-item.result-pending');
    const retrying = document.querySelectorAll('.result-item .delivery-icon');
    let allSettled = true;
    document.querySelectorAll('.result-item').forEach(row => {
        if (row.classList.contains('result-pending')) allSettled = false;
        const ic = row.querySelector('.delivery-icon');
        if (ic && ic.textContent === '🔄') allSettled = false;
    });
    if (allSettled) {
        if (currentSSE) { currentSSE.close(); currentSSE = null; }
        document.getElementById('post-again-btn').classList.remove('hidden');
    }
}
```

- [ ] **Step 4: Add delivery status CSS to style.css**

Append to `frontend/css/style.css`:

```css
.result-pending {
    background: color-mix(in srgb, var(--muted-color) 10%, transparent);
    border: 1px solid var(--muted-border-color);
}

.delivery-icon {
    margin-right: 0.4rem;
}

.delivery-status {
    color: var(--muted-color);
}

.result-success .delivery-status a {
    font-size: 0.875rem;
}
```

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html frontend/js/app.js frontend/css/style.css
git commit -m "feat: async compose flow with per-account SSE status updates"
```

---

## Task 11: Add History page

**Files:**
- Create: `frontend/history.html`
- Create: `frontend/js/history.js`
- Modify: `frontend/profile.html`

- [ ] **Step 1: Create history.html**

Create `frontend/history.html`:

```html
<!DOCTYPE html>
<html lang="en" data-theme="light">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>History - EchoBridge</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css">
    <link rel="stylesheet" href="css/style.css">
</head>
<body>
    <header class="container">
        <nav>
            <ul>
                <li><strong>EchoBridge</strong></li>
            </ul>
            <ul>
                <li><a href="index.html">Compose</a></li>
                <li><a href="history.html" class="contrast">History</a></li>
                <li><a href="profile.html">Accounts</a></li>
            </ul>
        </nav>
    </header>

    <main class="container">
        <article>
            <header>
                <h2>Post History</h2>
            </header>
            <div id="jobs-list">
                <p><em>Loading history...</em></p>
            </div>
        </article>
    </main>

    <footer class="container">
        <small>EchoBridge - Cross-post to multiple platforms</small>
    </footer>

    <script src="config.js"></script>
    <script src="js/history.js"></script>
</body>
</html>
```

- [ ] **Step 2: Create history.js**

Create `frontend/js/history.js`:

```javascript
const API_BASE = (window.ECHOBRIDGE_CONFIG && window.ECHOBRIDGE_CONFIG.apiBase) || '/api';

let globalSSE = null;

document.addEventListener('DOMContentLoaded', () => {
    loadJobs();
    subscribeGlobal();
});

async function loadJobs() {
    const container = document.getElementById('jobs-list');
    try {
        const res = await fetch(`${API_BASE}/posts`);
        const jobs = await res.json();
        if (!jobs || jobs.length === 0) {
            container.innerHTML = '<p class="no-accounts">No posts yet. <a href="index.html">Compose one</a></p>';
            return;
        }
        container.innerHTML = jobs.map(renderJobCard).join('');
    } catch (e) {
        container.innerHTML = '<p class="no-accounts">Failed to load history</p>';
    }
}

function renderJobCard(job) {
    const date = new Date(job.created_at).toLocaleString();
    const preview = job.content.length > 120 ? job.content.slice(0, 120) + '…' : job.content;
    const deliveriesHTML = (job.deliveries || []).map(d => renderDeliveryRow(d)).join('');
    return `<article class="job-card" id="job-${job.id}">
        <header>
            <small>${date}</small>
            <span class="job-status job-status--${job.status}">${job.status}</span>
        </header>
        <p class="job-content">${escapeHTML(preview)}</p>
        <div class="deliveries" id="deliveries-${job.id}">${deliveriesHTML}</div>
    </article>`;
}

function renderDeliveryRow(d) {
    const icon = deliveryIcon(d.status);
    const retryBtn = d.status === 'permanent_fail'
        ? `<button class="retry-btn outline" onclick="retryJob('${d.job_id}')" type="button">Retry</button>`
        : '';
    const link = d.post_url ? `<a href="${d.post_url}" target="_blank">View</a>` : '';
    const errMsg = d.error ? `<small class="error-msg">${escapeHTML(d.error)}</small>` : '';
    return `<div class="delivery-row" id="delivery-row-${d.id}" data-delivery-id="${d.id}">
        <span class="delivery-icon">${icon}</span>
        <span class="delivery-name">${escapeHTML(d.display_name)}</span>
        ${link}
        ${errMsg}
        ${retryBtn}
    </div>`;
}

function deliveryIcon(status) {
    switch (status) {
        case 'delivered': return '✅';
        case 'permanent_fail': return '❌';
        case 'processing': return '⏳';
        default: return '🔄';
    }
}

function subscribeGlobal() {
    if (globalSSE) globalSSE.close();
    globalSSE = new EventSource(`${API_BASE}/events`);
    globalSSE.onmessage = (e) => {
        const evt = JSON.parse(e.data);
        updateDeliveryRow(evt);
    };
    globalSSE.onerror = () => { globalSSE.close(); globalSSE = null; };
}

function updateDeliveryRow(evt) {
    const rows = document.querySelectorAll(`[data-delivery-id="${evt.delivery_id}"]`);
    if (rows.length === 0) return;
    rows.forEach(row => {
        const icon = row.querySelector('.delivery-icon');
        if (icon) icon.textContent = deliveryIcon(evt.status);

        // Update retry button presence
        const existing = row.querySelector('.retry-btn');
        if (evt.status === 'permanent_fail' && !existing) {
            row.insertAdjacentHTML('beforeend', `<button class="retry-btn outline" onclick="retryJob('${evt.job_id}')" type="button">Retry</button>`);
        } else if (evt.status !== 'permanent_fail' && existing) {
            existing.remove();
        }

        // Update view link
        if (evt.post_url) {
            const existingLink = row.querySelector('a');
            if (!existingLink) {
                row.insertAdjacentHTML('beforeend', `<a href="${evt.post_url}" target="_blank">View</a>`);
            }
        }
    });
}

async function retryJob(jobID) {
    try {
        const res = await fetch(`${API_BASE}/posts/${jobID}/retry`, { method: 'POST' });
        if (!res.ok) throw new Error('Retry failed');
        // Reload the card for this job
        const job = await res.json();
        const card = document.getElementById(`job-${jobID}`);
        if (card) card.outerHTML = renderJobCard(job);
    } catch (e) {
        alert('Failed to retry: ' + e.message);
    }
}

function escapeHTML(str) {
    return str.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}
```

- [ ] **Step 3: Add History link to profile.html nav**

In `frontend/profile.html`, replace the nav `<ul>`:

```html
            <ul>
                <li><a href="index.html">Compose</a></li>
                <li><a href="history.html">History</a></li>
                <li><a href="profile.html" class="contrast">Accounts</a></li>
            </ul>
```

- [ ] **Step 4: Add history CSS to style.css**

Append to `frontend/css/style.css`:

```css
.job-card {
    margin-bottom: 1rem;
}

.job-card header {
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.job-content {
    font-size: 0.95rem;
    white-space: pre-wrap;
    margin-bottom: 0.75rem;
}

.job-status {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
}

.job-status--completed { background: color-mix(in srgb, var(--ins-color) 20%, transparent); color: var(--ins-color); }
.job-status--partial   { background: color-mix(in srgb, #e67e22 20%, transparent); color: #e67e22; }
.job-status--failed    { background: color-mix(in srgb, var(--del-color) 20%, transparent); color: var(--del-color); }
.job-status--pending,
.job-status--processing { background: color-mix(in srgb, var(--muted-color) 15%, transparent); color: var(--muted-color); }

.deliveries {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
}

.delivery-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
}

.delivery-name {
    flex: 1;
}

.error-msg {
    color: var(--del-color);
}

.retry-btn {
    padding: 0.2rem 0.6rem;
    font-size: 0.75rem;
    margin: 0;
}
```

- [ ] **Step 5: Commit**

```bash
git add frontend/history.html frontend/js/history.js frontend/profile.html frontend/css/style.css
git commit -m "feat: add history page with per-platform delivery status and retry"
```

---

## Task 12: Final integration check

- [ ] **Step 1: Run full test suite**

Run: `cd /home/sathyabhat/code/echobridge/backend && go test ./... -timeout 60s -v 2>&1 | tail -40`
Expected: all PASS, no failures

- [ ] **Step 2: Run go vet**

Run: `cd /home/sathyabhat/code/echobridge/backend && go vet ./...`
Expected: no output (no issues)

- [ ] **Step 3: Build the binary**

Run: `cd /home/sathyabhat/code/echobridge && make build`
Expected: `backend/echobridge` binary created with no errors

- [ ] **Step 4: Commit if any fixes were needed**

If any build or vet issues were fixed above, commit them:

```bash
git add -p
git commit -m "fix: address any build or vet issues found in integration check"
```

---

## Self-Review Notes

- **Spec coverage:**
  - ✅ Async `POST /api/posts` → returns `job_id`
  - ✅ `GET /api/posts` history (all, no pagination)
  - ✅ `GET /api/posts/{id}` single job
  - ✅ `POST /api/posts/{id}/retry` manual retry
  - ✅ `GET /api/events` global SSE
  - ✅ `GET /api/events/{jobID}` per-job SSE
  - ✅ Worker pool with configurable `ECHOBRIDGE_WORKERS`
  - ✅ Exponential backoff, max 10 attempts → `permanent_fail`
  - ✅ 4xx = permanent fail, 5xx / network = transient
  - ✅ Crash recovery on startup
  - ✅ Retry scheduler (30s tick)
  - ✅ Manual retry resets `permanent_fail` deliveries only
  - ✅ Partial job status (some delivered, some failed)
  - ✅ Per-platform checkmarks / spinner / X in compose page
  - ✅ History page with live SSE updates
  - ✅ Bluesky token refresh in workers
  - ✅ Media upload per delivery in workers
  - ✅ History nav link added to all pages

- **Type consistency:** `workerProvider`/`WorkerProvider`, `providerFactory`/`ProviderFactory`, `TokenRefreshFunc` are consistent across queue.go, queue_test.go, and main.go.
