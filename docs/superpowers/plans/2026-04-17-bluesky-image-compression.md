# Bluesky Image Compression Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compress images exceeding Bluesky's 2MB blob limit before uploading, so large image uploads succeed instead of failing with "blob too big".

**Architecture:** A new `compressImage()` function in `bluesky.go` decodes the image (JPEG/PNG/GIF via stdlib, WebP via `golang.org/x/image/webp`) and re-encodes as JPEG at decreasing quality levels until under 2MB. `UploadMedia()` calls it unconditionally after reading the file, before sending to the PDS.

**Tech Stack:** Go stdlib (`image`, `image/jpeg`, `image/png`, `image/gif`), `golang.org/x/image/webp`.

---

## File Map

| File | Change |
|------|--------|
| `backend/go.mod` / `backend/go.sum` | Add `golang.org/x/image` dependency |
| `backend/internal/providers/bluesky.go` | Add `compressImage()`, wire into `UploadMedia()` |
| `backend/internal/providers/bluesky_test.go` | Add tests for `compressImage()` |

---

### Task 1: Add `golang.org/x/image` dependency

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd backend && go get golang.org/x/image/webp
```

Expected: `golang.org/x/image` added to `go.mod`.

- [ ] **Step 2: Verify build**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd .. && git add backend/go.mod backend/go.sum
git commit -m "chore: add golang.org/x/image for WebP decoding"
```

---

### Task 2: Implement `compressImage()`

**Files:**
- Modify: `backend/internal/providers/bluesky.go`
- Modify: `backend/internal/providers/bluesky_test.go`

- [ ] **Step 1: Write failing tests**

Add to `backend/internal/providers/bluesky_test.go`, before the `// --- atURIToURL ---` comment:

```go
// --- compressImage ---

func TestCompressImage_UnderLimit_Passthrough(t *testing.T) {
	// A small valid JPEG — must be under 2MB, should be returned unchanged.
	data := makeTestJPEG(t, 100, 100)
	if len(data) >= 2_000_000 {
		t.Skip("test image unexpectedly large")
	}
	got, gotCT, err := compressImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("compressImage: %v", err)
	}
	if gotCT != "image/jpeg" {
		t.Errorf("content type: got %q, want %q", gotCT, "image/jpeg")
	}
	if !bytes.Equal(got, data) {
		t.Error("expected data to be returned unchanged when under limit")
	}
}

func TestCompressImage_JPEG_OverLimit_Compressed(t *testing.T) {
	// Build a large JPEG by repeating a small one until it exceeds 2MB.
	small := makeTestJPEG(t, 200, 200)
	data := makeLargeImage(t, small, 2_100_000)

	got, gotCT, err := compressImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("compressImage: %v", err)
	}
	if gotCT != "image/jpeg" {
		t.Errorf("content type: got %q, want %q", gotCT, "image/jpeg")
	}
	if len(got) >= 2_000_000 {
		t.Errorf("compressed size %d still exceeds 2MB", len(got))
	}
}

func TestCompressImage_PNG_OverLimit_ReencodedAsJPEG(t *testing.T) {
	data := makeOversizedPNG(t, 2_100_000)

	got, gotCT, err := compressImage(data, "image/png")
	if err != nil {
		t.Fatalf("compressImage: %v", err)
	}
	if gotCT != "image/jpeg" {
		t.Errorf("content type: got %q, want %q", gotCT, "image/jpeg")
	}
	if len(got) >= 2_000_000 {
		t.Errorf("compressed size %d still exceeds 2MB", len(got))
	}
}

func TestCompressImage_DecodeFailure_ReturnsOriginal(t *testing.T) {
	corrupt := []byte("this is not a valid image")
	got, gotCT, err := compressImage(corrupt, "image/jpeg")
	if err != nil {
		t.Fatalf("expected no error on decode failure, got: %v", err)
	}
	if !bytes.Equal(got, corrupt) {
		t.Error("expected original data returned on decode failure")
	}
	if gotCT != "image/jpeg" {
		t.Errorf("content type: got %q, want %q", gotCT, "image/jpeg")
	}
}
```

Also add these test helpers after the existing `blueskyAccount` helper:

```go
// makeTestJPEG creates a minimal valid JPEG of the given pixel dimensions.
func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("makeTestJPEG: %v", err)
	}
	return buf.Bytes()
}

// makeLargeImage returns data of at least minSize bytes by tiling the given
// JPEG into a tall image and re-encoding it.
func makeLargeImage(t *testing.T, _ []byte, minSize int) []byte {
	t.Helper()
	// Create a large enough NRGBA image to produce a JPEG > minSize when encoded
	// at high quality.
	side := 2000
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	// Fill with non-uniform data so JPEG can't compress it trivially.
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 99}); err != nil {
		t.Fatalf("makeLargeImage encode: %v", err)
	}
	if buf.Len() < minSize {
		t.Skipf("could not generate image of size %d (got %d) — skipping", minSize, buf.Len())
	}
	return buf.Bytes()
}

// makeOversizedPNG creates a PNG that encodes to at least minSize bytes.
func makeOversizedPNG(t *testing.T, minSize int) []byte {
	t.Helper()
	side := 2000
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * y), G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("makeOversizedPNG: %v", err)
	}
	if buf.Len() < minSize {
		t.Skipf("could not generate PNG of size %d (got %d) — skipping", minSize, buf.Len())
	}
	return buf.Bytes()
}
```

Add the following imports to `bluesky_test.go` (replace existing import block):

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sathyabhat/echobridge/internal/models"
)
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd backend && go test ./internal/providers/ -run "TestCompressImage" -v
```

Expected: compile error — `compressImage undefined`.

- [ ] **Step 3: Implement `compressImage()`**

Add the following to `backend/internal/providers/bluesky.go` in the `// --- Helpers ---` section, before `fetchLinkCard`:

First, add these imports to the import block in `bluesky.go`:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/image/webp"

	"github.com/sathyabhat/echobridge/internal/models"
)
```

Then add the function in the `// --- Helpers ---` section before `fetchLinkCard`:

```go
const blueskyMaxBlobSize = 2_000_000

// compressImage returns data ready to upload to Bluesky. If data is already
// under 2MB it is returned unchanged. Otherwise the image is decoded and
// re-encoded as JPEG at decreasing quality levels until it fits. On decode
// failure the original data and contentType are returned unchanged so the
// caller can let the upload fail with the platform's own error message.
func compressImage(data []byte, contentType string) ([]byte, string, error) {
	if len(data) <= blueskyMaxBlobSize {
		return data, contentType, nil
	}

	var img image.Image
	var err error

	if contentType == "image/webp" {
		img, err = webp.Decode(bytes.NewReader(data))
	} else {
		img, _, err = image.Decode(bytes.NewReader(data))
	}
	if err != nil {
		// Corrupt or unsupported format — return original and let upload fail.
		return data, contentType, nil
	}

	var best []byte
	for _, quality := range []int{85, 75, 60, 50} {
		var buf bytes.Buffer
		if encErr := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); encErr != nil {
			continue
		}
		best = buf.Bytes()
		if len(best) <= blueskyMaxBlobSize {
			break
		}
	}

	if best == nil {
		return data, contentType, nil
	}
	return best, "image/jpeg", nil
}
```

- [ ] **Step 4: Run the tests**

```bash
cd backend && go test ./internal/providers/ -run "TestCompressImage" -v
```

Expected: all four tests pass.

- [ ] **Step 5: Run full suite**

```bash
cd backend && go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
cd .. && git add backend/internal/providers/bluesky.go backend/internal/providers/bluesky_test.go
git commit -m "feat: add compressImage to re-encode oversized images as JPEG"
```

---

### Task 3: Wire `compressImage()` into `UploadMedia()`

**Files:**
- Modify: `backend/internal/providers/bluesky.go`
- Modify: `backend/internal/providers/bluesky_test.go`

- [ ] **Step 1: Write a failing integration test**

Add to `backend/internal/providers/bluesky_test.go` before `// --- compressImage ---`:

```go
// --- UploadMedia compression integration ---

func TestUploadMedia_CompressesOversizedImage(t *testing.T) {
	var uploadedSize int
	var uploadedCT string

	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		uploadedSize = len(body)
		uploadedCT = r.Header.Get("Content-Type")
		json.NewEncoder(w).Encode(blueskyUploadBlobResponse{
			Blob: blueskyBlob{
				Type:     "blob",
				Ref:      blobRef{Link: "bafy123"},
				MimeType: uploadedCT,
				Size:     int64(uploadedSize),
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Generate an oversized JPEG.
	imgData := makeLargeImage(t, nil, 2_100_000)

	b := NewBlueskyWithClient(srv.Client())
	acc := blueskyAccount(srv.URL)

	_, err := b.UploadMedia(context.Background(), acc,
		bytes.NewReader(imgData), "photo.jpg", "image/jpeg")
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if uploadedSize >= 2_000_000 {
		t.Errorf("uploaded size %d still exceeds 2MB limit", uploadedSize)
	}
	if uploadedCT != "image/jpeg" {
		t.Errorf("uploaded content type: got %q, want %q", uploadedCT, "image/jpeg")
	}
}
```

Add `"io"` to the imports in `bluesky_test.go` (add to the existing import block):

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sathyabhat/echobridge/internal/models"
)
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd backend && go test ./internal/providers/ -run "TestUploadMedia_CompressesOversizedImage" -v
```

Expected: FAIL — uploaded size is still over 2MB (compression not wired yet).

- [ ] **Step 3: Wire `compressImage()` into `UploadMedia()`**

In `backend/internal/providers/bluesky.go`, update `UploadMedia()`. Replace:

```go
	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pdsURL+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", contentType)
```

With:

```go
	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	data, contentType, _ = compressImage(data, contentType)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pdsURL+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", contentType)
```

- [ ] **Step 4: Run integration test**

```bash
cd backend && go test ./internal/providers/ -run "TestUploadMedia_CompressesOversizedImage" -v
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```bash
cd backend && go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
cd .. && git add backend/internal/providers/bluesky.go backend/internal/providers/bluesky_test.go
git commit -m "feat: compress oversized images in UploadMedia before sending to Bluesky"
```

---

### Task 4: Push

- [ ] **Step 1: Push to remote**

```bash
git push
```
