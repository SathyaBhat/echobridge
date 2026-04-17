# Bluesky Link Preview Cards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a Bluesky post contains a URL and no attached images, fetch its Open Graph metadata and attach an `app.bsky.embed.external` embed (link card) with title, description, and optional thumbnail.

**Architecture:** `fetchLinkCard()` on the `Bluesky` struct fetches the target URL, parses OG tags with `golang.org/x/net/html`, uploads any `og:image` as a PDS blob, and returns a `*blueskyExternalEmbed`. `Post()` calls it when no images are attached. All failures are silent — the post always goes through.

**Tech Stack:** Go, `golang.org/x/net/html`, `net/http/httptest` for tests.

---

## File Map

| File | Change |
|------|--------|
| `backend/go.mod` / `backend/go.sum` | Add `golang.org/x/net` dependency |
| `backend/internal/providers/bluesky.go` | Add structs, `fetchLinkCard()`, wire into `Post()`, change `Embed` field to `any` |
| `backend/internal/providers/bluesky_test.go` | Add tests for `fetchLinkCard()` and updated `Post()` paths |

---

### Task 1: Add `golang.org/x/net` dependency

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd backend && go get golang.org/x/net/html
```

Expected output: line added to `go.mod` like `golang.org/x/net v0.x.x`

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore: add golang.org/x/net for HTML parsing"
```

---

### Task 2: Add new structs and change `Embed` field type

**Files:**
- Modify: `backend/internal/providers/bluesky.go`

- [ ] **Step 1: Write a failing test that imports the new types**

Add to `backend/internal/providers/bluesky_test.go` before the `// --- atURIToURL ---` comment:

```go
// --- fetchLinkCard structs compile check ---

func TestBlueskyExternalEmbedStructs(t *testing.T) {
	card := blueskyExternalEmbed{
		Type: "app.bsky.embed.external",
		External: blueskyLinkCard{
			URI:         "https://example.com",
			Title:       "Example",
			Description: "A page",
			Thumb:       nil,
		},
	}
	if card.Type != "app.bsky.embed.external" {
		t.Errorf("unexpected type: %s", card.Type)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails (types don't exist yet)**

```bash
cd backend && go test ./internal/providers/ -run TestBlueskyExternalEmbedStructs -v
```

Expected: compile error — `blueskyExternalEmbed undefined`.

- [ ] **Step 3: Add the new structs and change `Embed` to `any`**

In `backend/internal/providers/bluesky.go`, replace the `blueskyPostRecord` struct and add new types after `blobRef`:

Replace:
```go
type blueskyPostRecord struct {
	Type      string          `json:"$type"`
	Text      string          `json:"text"`
	CreatedAt string          `json:"createdAt"`
	Facets    []blueskyFacet  `json:"facets,omitempty"`
	Embed     *blueskyEmbed   `json:"embed,omitempty"`
}
```

With:
```go
type blueskyPostRecord struct {
	Type      string         `json:"$type"`
	Text      string         `json:"text"`
	CreatedAt string         `json:"createdAt"`
	Facets    []blueskyFacet `json:"facets,omitempty"`
	Embed     any            `json:"embed,omitempty"`
}
```

Add after the `blobRef` struct:

```go
// blueskyExternalEmbed is the app.bsky.embed.external embed for link cards.
type blueskyExternalEmbed struct {
	Type     string          `json:"$type"` // "app.bsky.embed.external"
	External blueskyLinkCard `json:"external"`
}

type blueskyLinkCard struct {
	URI         string       `json:"uri"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Thumb       *blueskyBlob `json:"thumb,omitempty"`
}
```

Also fix the existing images embed assignment in `Post()` — change:
```go
record.Embed = &blueskyEmbed{
    Type:   "app.bsky.embed.images",
    Images: images,
}
```
to:
```go
record.Embed = blueskyEmbed{
    Type:   "app.bsky.embed.images",
    Images: images,
}
```
(No pointer needed now that `Embed` is `any`.)

- [ ] **Step 4: Run the compile-check test**

```bash
cd backend && go test ./internal/providers/ -run TestBlueskyExternalEmbedStructs -v
```

Expected: `PASS`.

- [ ] **Step 5: Run full suite to check no regressions**

```bash
cd backend && go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/providers/bluesky.go backend/internal/providers/bluesky_test.go
git commit -m "feat: add blueskyExternalEmbed structs, change Embed field to any"
```

---

### Task 3: Implement `fetchLinkCard()`

**Files:**
- Modify: `backend/internal/providers/bluesky.go`
- Modify: `backend/internal/providers/bluesky_test.go`

- [ ] **Step 1: Write failing tests for `fetchLinkCard()`**

Add to `backend/internal/providers/bluesky_test.go` before `// --- atURIToURL ---`:

```go
// --- fetchLinkCard ---

func TestFetchLinkCard_OGTags(t *testing.T) {
	// Serve a page with full OG tags; no og:image so no blob upload needed.
	ogMux := http.NewServeMux()
	ogMux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head>
<meta property="og:title" content="OG Title"/>
<meta property="og:description" content="OG Desc"/>
</head><body></body></html>`)
	})
	ogSrv := httptest.NewServer(ogMux)
	defer ogSrv.Close()

	b := NewBlueskyWithClient(ogSrv.Client())
	card, err := b.fetchLinkCard(context.Background(), "", "", ogSrv.URL+"/page")
	if err != nil {
		t.Fatalf("fetchLinkCard: %v", err)
	}
	if card.External.Title != "OG Title" {
		t.Errorf("Title: got %q, want %q", card.External.Title, "OG Title")
	}
	if card.External.Description != "OG Desc" {
		t.Errorf("Description: got %q, want %q", card.External.Description, "OG Desc")
	}
	if card.External.URI != ogSrv.URL+"/page" {
		t.Errorf("URI: got %q", card.External.URI)
	}
	if card.Thumb != nil {
		t.Error("expected no thumb when og:image absent")
	}
}

func TestFetchLinkCard_FallbackTitle(t *testing.T) {
	// No og:title — should fall back to <title> element.
	ogMux := http.NewServeMux()
	ogMux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Page Title</title></head><body></body></html>`)
	})
	ogSrv := httptest.NewServer(ogMux)
	defer ogSrv.Close()

	b := NewBlueskyWithClient(ogSrv.Client())
	card, err := b.fetchLinkCard(context.Background(), "", "", ogSrv.URL+"/page")
	if err != nil {
		t.Fatalf("fetchLinkCard: %v", err)
	}
	if card.External.Title != "Page Title" {
		t.Errorf("Title: got %q, want %q", card.External.Title, "Page Title")
	}
}

func TestFetchLinkCard_WithThumbnail(t *testing.T) {
	// og:image present — should upload blob and set Thumb.
	blobData := blueskyBlob{
		Type:     "blob",
		Ref:      blobRef{Link: "bafkreithumb"},
		MimeType: "image/jpeg",
		Size:     100,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// og:image points to /img on the same test server
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, "REPLACE_WITH_SERVER_URL")
	})
	mux.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fakejpeg"))
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(blueskyUploadBlobResponse{Blob: blobData})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Re-serve the page with the correct server URL substituted in.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, srv.URL)
	})
	mux2.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fakejpeg"))
	})
	mux2.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(blueskyUploadBlobResponse{Blob: blobData})
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()

	b := NewBlueskyWithClient(srv2.Client())
	card, err := b.fetchLinkCard(context.Background(), srv2.URL, "token", srv2.URL+"/page")
	if err != nil {
		t.Fatalf("fetchLinkCard: %v", err)
	}
	if card.Thumb == nil {
		t.Fatal("expected Thumb to be set")
	}
	if card.Thumb.Ref.Link != "bafkreithumb" {
		t.Errorf("Thumb ref: got %q, want %q", card.Thumb.Ref.Link, "bafkreithumb")
	}
}

func TestFetchLinkCard_ThumbUploadFails_CardStillReturned(t *testing.T) {
	// og:image present but blob upload returns 500 — card returned without Thumb.
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, "REPLACED")
	})
	mux.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fakejpeg"))
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux2 := http.NewServeMux()
	mux2.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, srv.URL)
	})
	mux2.HandleFunc("/img", mux.Handler(mustParseRequest("/img")))
	mux2.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", mux.Handler(mustParseRequest("/xrpc/com.atproto.repo.uploadBlob")))
	// Use a simpler single-server approach:
	mux3 := http.NewServeMux()
	var srv3 *httptest.Server
	mux3.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, srv3.URL)
	})
	mux3.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fakejpeg"))
	})
	mux3.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	srv3 = httptest.NewServer(mux3)
	defer srv3.Close()

	b := NewBlueskyWithClient(srv3.Client())
	card, err := b.fetchLinkCard(context.Background(), srv3.URL, "token", srv3.URL+"/page")
	if err != nil {
		t.Fatalf("fetchLinkCard: %v", err)
	}
	if card.External.Title != "Title" {
		t.Errorf("Title: got %q, want %q", card.External.Title, "Title")
	}
	if card.Thumb != nil {
		t.Error("expected no Thumb when upload fails")
	}
}

func TestFetchLinkCard_ServerError_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := NewBlueskyWithClient(srv.Client())
	_, err := b.fetchLinkCard(context.Background(), "", "", srv.URL+"/page")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}
```

Also add this helper at the top of the test file (after the imports):

```go
func mustParseRequest(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
}
```

- [ ] **Step 2: Add `"fmt"` to test file imports if not already present**

The test file already imports `"encoding/json"`, `"net/http"`, `"net/http/httptest"`, `"strings"`, `"context"`, `"testing"`. Add `"fmt"` to the import block.

- [ ] **Step 3: Run tests to confirm they fail**

```bash
cd backend && go test ./internal/providers/ -run "TestFetchLinkCard" -v
```

Expected: compile error — `fetchLinkCard undefined`.

- [ ] **Step 4: Implement `fetchLinkCard()`**

Add to `backend/internal/providers/bluesky.go`, in the `// --- Helpers ---` section before `atURIToURL`:

```go
// fetchLinkCard fetches uri, parses Open Graph metadata, and returns a link
// card embed. pdsURL and accessToken are used only if an og:image needs to be
// uploaded as a thumbnail blob. Returns an error if the page fetch fails; thumb
// upload failures are silent (card returned without Thumb).
func (b *Bluesky) fetchLinkCard(ctx context.Context, pdsURL, accessToken, uri string) (*blueskyExternalEmbed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EchoBridge/1.0)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchLinkCard: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetchLinkCard: %s returned %s", uri, resp.Status)
	}

	title, description, imageURL := parseOGTags(resp.Body)

	card := &blueskyExternalEmbed{
		Type: "app.bsky.embed.external",
		External: blueskyLinkCard{
			URI:         uri,
			Title:       title,
			Description: description,
		},
	}

	if imageURL != "" && pdsURL != "" {
		thumb, err := b.fetchAndUploadThumb(ctx, pdsURL, accessToken, imageURL)
		if err == nil {
			card.External.Thumb = thumb
		}
		// thumb upload failure is intentionally silent
	}

	return card, nil
}

// parseOGTags reads HTML from r and extracts og:title (fallback: <title>),
// og:description (fallback: meta[name=description]), and og:image.
func parseOGTags(r io.Reader) (title, description, imageURL string) {
	doc, err := html.Parse(r)
	if err != nil {
		return
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				prop := attrVal(n, "property")
				name := attrVal(n, "name")
				content := attrVal(n, "content")
				switch prop {
				case "og:title":
					title = content
				case "og:description":
					description = content
				case "og:image":
					imageURL = content
				}
				if name == "description" && description == "" {
					description = content
				}
			case "title":
				if title == "" && n.FirstChild != nil {
					title = n.FirstChild.Data
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return
}

// attrVal returns the value of the named attribute on n, or "".
func attrVal(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// fetchAndUploadThumb downloads imageURL and uploads it to the PDS as a blob.
func (b *Bluesky) fetchAndUploadThumb(ctx context.Context, pdsURL, accessToken, imageURL string) (*blueskyBlob, error) {
	imgResp, err := b.httpClient.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image fetch %s: %s", imageURL, imgResp.Status)
	}

	data, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return nil, err
	}

	contentType := imgResp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pdsURL+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", contentType)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uploadBlob: %s", resp.Status)
	}

	var uploadResp blueskyUploadBlobResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, err
	}
	return &uploadResp.Blob, nil
}
```

Add `"golang.org/x/net/html"` to the import block in `bluesky.go`.

- [ ] **Step 5: Remove the `mustParseRequest` helper and simplify `TestFetchLinkCard_ThumbUploadFails_CardStillReturned`**

The test for thumb-upload-failure was written with some intermediate complexity. Replace the entire `TestFetchLinkCard_ThumbUploadFails_CardStillReturned` test with this cleaner version:

```go
func TestFetchLinkCard_ThumbUploadFails_CardStillReturned(t *testing.T) {
	var srv3 *httptest.Server
	mux3 := http.NewServeMux()
	mux3.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, srv3.URL)
	})
	mux3.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fakejpeg"))
	})
	mux3.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	srv3 = httptest.NewServer(mux3)
	defer srv3.Close()

	b := NewBlueskyWithClient(srv3.Client())
	card, err := b.fetchLinkCard(context.Background(), srv3.URL, "token", srv3.URL+"/page")
	if err != nil {
		t.Fatalf("fetchLinkCard: %v", err)
	}
	if card.External.Title != "Title" {
		t.Errorf("Title: got %q, want %q", card.External.Title, "Title")
	}
	if card.Thumb != nil {
		t.Error("expected no Thumb when upload fails")
	}
}
```

Also remove the `mustParseRequest` helper — it is no longer needed.

- [ ] **Step 6: Run the fetchLinkCard tests**

```bash
cd backend && go test ./internal/providers/ -run "TestFetchLinkCard" -v
```

Expected: all pass.

- [ ] **Step 7: Run the full suite**

```bash
cd backend && go test ./...
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/providers/bluesky.go backend/internal/providers/bluesky_test.go
git commit -m "feat: implement fetchLinkCard with OG parsing and thumbnail upload"
```

---

### Task 4: Wire `fetchLinkCard()` into `Post()`

**Files:**
- Modify: `backend/internal/providers/bluesky.go`
- Modify: `backend/internal/providers/bluesky_test.go`

- [ ] **Step 1: Write failing integration tests**

Add to `backend/internal/providers/bluesky_test.go` before `// --- atURIToURL ---`:

```go
// --- Post() link card integration ---

func TestBlueskyPost_LinkCardAttached_WhenNoImages(t *testing.T) {
	var capturedRecord blueskyPostRecord
	var srv *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head>
<meta property="og:title" content="Link Title"/>
<meta property="og:description" content="Link Desc"/>
</head><body></body></html>`)
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		var req blueskyCreateRecordRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.Unmarshal(req.Record, &capturedRecord)
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{
			URI: "at://did:plc:testdid/app.bsky.feed.post/rkey123",
		})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	acc := blueskyAccount(srv.URL)
	b := NewBlueskyWithClient(srv.Client())

	result, err := b.Post(context.Background(), acc,
		"Check out "+srv.URL+"/page", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	embedJSON, err := json.Marshal(capturedRecord.Embed)
	if err != nil {
		t.Fatalf("marshal embed: %v", err)
	}
	var embed blueskyExternalEmbed
	if err := json.Unmarshal(embedJSON, &embed); err != nil {
		t.Fatalf("unmarshal embed: %v", err)
	}
	if embed.Type != "app.bsky.embed.external" {
		t.Errorf("embed type: got %q, want %q", embed.Type, "app.bsky.embed.external")
	}
	if embed.External.Title != "Link Title" {
		t.Errorf("card title: got %q, want %q", embed.External.Title, "Link Title")
	}
}

func TestBlueskyPost_NoLinkCard_WhenImagesPresent(t *testing.T) {
	var capturedRecord blueskyPostRecord
	var srv *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		// This should NOT be called when images are present.
		t.Error("fetchLinkCard should not be called when images are present")
		http.Error(w, "unexpected call", http.StatusInternalServerError)
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		var req blueskyCreateRecordRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.Unmarshal(req.Record, &capturedRecord)
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{
			URI: "at://did:plc:testdid/app.bsky.feed.post/rkey123",
		})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	blob := blueskyBlob{Type: "blob", Ref: blobRef{Link: "bafy"}, MimeType: "image/jpeg", Size: 1}
	blobJSON, _ := json.Marshal(blob)

	acc := blueskyAccount(srv.URL)
	b := NewBlueskyWithClient(srv.Client())

	result, err := b.Post(context.Background(), acc,
		"Check out "+srv.URL+"/page", []string{string(blobJSON)})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success: %s", result.Error)
	}

	embedJSON, _ := json.Marshal(capturedRecord.Embed)
	if strings.Contains(string(embedJSON), "embed.external") {
		t.Error("expected no external embed when images present")
	}
}

func TestBlueskyPost_LinkCardFetchFails_PostSucceeds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{
			URI: "at://did:plc:testdid/app.bsky.feed.post/rkey123",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	acc := blueskyAccount(srv.URL)
	b := NewBlueskyWithClient(srv.Client())

	result, err := b.Post(context.Background(), acc,
		"Check out "+srv.URL+"/page", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected post to succeed even when card fetch fails: %s", result.Error)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd backend && go test ./internal/providers/ -run "TestBlueskyPost_LinkCard|TestBlueskyPost_NoLinkCard" -v
```

Expected: `TestBlueskyPost_LinkCardAttached_WhenNoImages` fails (no card attached yet); others may pass vacuously.

- [ ] **Step 3: Wire `fetchLinkCard()` into `Post()`**

In `backend/internal/providers/bluesky.go`, update the embed section of `Post()`. Replace the existing block:

```go
	// mediaIDs are JSON-encoded blueskyBlob values returned by UploadMedia.
	if len(mediaIDs) > 0 {
		var images []blueskyImage
		for _, blobJSON := range mediaIDs {
			var blob blueskyBlob
			if err := json.Unmarshal([]byte(blobJSON), &blob); err == nil {
				images = append(images, blueskyImage{Image: blob, Alt: ""})
			}
		}
		if len(images) > 0 {
			record.Embed = blueskyEmbed{
				Type:   "app.bsky.embed.images",
				Images: images,
			}
		}
	}
```

With:

```go
	// mediaIDs are JSON-encoded blueskyBlob values returned by UploadMedia.
	if len(mediaIDs) > 0 {
		var images []blueskyImage
		for _, blobJSON := range mediaIDs {
			var blob blueskyBlob
			if err := json.Unmarshal([]byte(blobJSON), &blob); err == nil {
				images = append(images, blueskyImage{Image: blob, Alt: ""})
			}
		}
		if len(images) > 0 {
			record.Embed = blueskyEmbed{
				Type:   "app.bsky.embed.images",
				Images: images,
			}
		}
	} else if urls := urlRe.FindAllString(content, 1); len(urls) > 0 {
		card, err := b.fetchLinkCard(ctx, pdsURL, account.AccessToken, urls[0])
		if err == nil {
			record.Embed = card
		}
		// card fetch failure is intentionally silent
	}
```

- [ ] **Step 4: Run integration tests**

```bash
cd backend && go test ./internal/providers/ -run "TestBlueskyPost_LinkCard|TestBlueskyPost_NoLinkCard" -v
```

Expected: all three pass.

- [ ] **Step 5: Run full suite**

```bash
cd backend && go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/providers/bluesky.go backend/internal/providers/bluesky_test.go
git commit -m "feat: attach link card embed when posting to Bluesky without images"
```

---

### Task 5: Push

- [ ] **Step 1: Push to remote**

```bash
git push
```
