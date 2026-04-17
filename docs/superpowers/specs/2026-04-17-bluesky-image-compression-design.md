# Bluesky Image Compression

**Date:** 2026-04-17  
**Status:** Approved

## Goal

Compress images that exceed Bluesky's 2MB blob limit before uploading, so posts with large images succeed instead of failing with "blob too big".

## Constraint

Bluesky enforces a hard 2,000,000 byte limit on uploaded blobs.

## Where the Change Lives

A new `compressImage(data []byte, contentType string) ([]byte, string, error)` function in `backend/internal/providers/bluesky.go`. `UploadMedia()` calls it unconditionally on every image upload, replacing `data` and `contentType` with the returned values before sending to the PDS.

## Compression Logic

1. If `len(data) <= 2_000_000` → return data and original content type unchanged (no-op)
2. Decode the image:
   - `image/jpeg`, `image/png`, `image/gif` → `image.Decode()` from stdlib
   - `image/webp` → `golang.org/x/image/webp` decoder
   - GIF: only the first frame is decoded and kept (animation not supported by Bluesky's limit anyway)
3. Re-encode as JPEG, trying quality levels `[]int{85, 75, 60, 50}` in order, stopping at the first result under 2MB
4. If all quality levels still exceed 2MB → return the 50-quality result anyway; let the upload fail with the platform's original error
5. Return compressed bytes and `"image/jpeg"`

## Error Handling

| Failure | Behaviour |
|---------|-----------|
| Decode fails (corrupt or unsupported format) | Return original data + content type unchanged |
| All quality levels exceed 2MB | Return quality-50 result, upload may fail with platform error |

## Dependencies

Add `golang.org/x/image` for WebP decoding (`golang.org/x/image/webp`). No other new dependencies — JPEG, PNG, and GIF are handled by Go stdlib.

## Testing

- Under-limit image: passthrough, data and content type unchanged
- JPEG over limit: re-encoded as JPEG, result under 2MB
- PNG over limit: re-encoded as JPEG, content type changed to `image/jpeg`
- WebP over limit: re-encoded as JPEG, content type changed to `image/jpeg`
- Decode failure: original data and content type returned unchanged
