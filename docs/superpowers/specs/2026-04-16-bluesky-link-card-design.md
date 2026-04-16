# Bluesky Link Preview Cards

**Date:** 2026-04-16  
**Status:** Approved

## Goal

When a Bluesky post contains a URL and no attached images, fetch the URL's Open Graph metadata and attach an `app.bsky.embed.external` embed (link card) with title, description, and thumbnail.

## Constraints

- Bluesky allows only one embed per post. If images are attached, skip the link card entirely.
- Card fetch failures must never block the post. The post always goes through.

## Data Flow in `Post()`

1. If `mediaIDs` is non-empty → build `app.bsky.embed.images` (existing path, unchanged)
2. Else if the post text contains a URL → call `fetchLinkCard()` with the first URL found
3. If `fetchLinkCard()` returns a card → attach as `Embed` on the post record
4. If `fetchLinkCard()` returns an error → log and continue without a card

## New Function: `fetchLinkCard()`

Signature: `(b *Bluesky) fetchLinkCard(ctx context.Context, pdsURL, accessToken, uri string) (*blueskyExternalEmbed, error)`

Steps:
1. GET the URL with a browser-like `User-Agent` header
2. Parse HTML response using `golang.org/x/net/html` for:
   - `og:title` → fallback to `<title>` element
   - `og:description` → fallback to `meta[name=description]`
   - `og:image` → if present, download the image and upload to PDS via `uploadBlob`
3. Return `*blueskyExternalEmbed` with populated fields; `Thumb` is omitted if image fetch/upload fails

## New Structs

```go
type blueskyExternalEmbed struct {
    Type     string          `json:"$type"`    // "app.bsky.embed.external"
    External blueskyLinkCard `json:"external"`
}

type blueskyLinkCard struct {
    URI         string       `json:"uri"`
    Title       string       `json:"title"`
    Description string       `json:"description"`
    Thumb       *blueskyBlob `json:"thumb,omitempty"`
}
```

`blueskyPostRecord.Embed` changes from `*blueskyEmbed` to `any` so it can hold either embed type.

## Error Handling

| Failure | Behaviour |
|---------|-----------|
| OG fetch timeout / non-200 | Post proceeds, no card |
| `og:image` download fails | Card included, no thumbnail |
| Redirect loop or unreachable URL | Post proceeds, no card |
| Any panic/unexpected error | Recover, post proceeds |

## Dependencies

Add `golang.org/x/net/html` for HTML parsing. No other new dependencies.

## Testing

- Unit test `fetchLinkCard()` against an `httptest.Server` serving mock HTML with OG tags
- Test fallback: no `og:title` → uses `<title>`
- Test thumbnail path: mock blob upload, verify `Thumb` is set
- Test image-upload failure: verify card is still returned without `Thumb`
- Test `Post()` integration: images present → no card; URL present, no images → card attached
- Test fetch failure → post still succeeds
