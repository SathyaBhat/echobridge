package providers

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

	"golang.org/x/image/webp"
	"golang.org/x/net/html"

	"github.com/sathyabhat/echobridge/internal/models"
)

var (
	hashtagRe = regexp.MustCompile(`#[a-zA-Z][a-zA-Z0-9_]*`)
	urlRe     = regexp.MustCompile(`https?://[^\s]+`)
)

const blueskyDefaultPDS = "https://bsky.social"

type Bluesky struct {
	httpClient *http.Client
}

func NewBluesky() *Bluesky {
	return &Bluesky{
		httpClient: &http.Client{},
	}
}

// NewBlueskyWithClient creates a Bluesky provider using the given HTTP client.
// Intended for testing.
func NewBlueskyWithClient(client *http.Client) *Bluesky {
	return &Bluesky{httpClient: client}
}

func (b *Bluesky) Name() string {
	return "bluesky"
}

// --- Session ---

type blueskySessionRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type BlueskySessionResponse struct {
	AccessJwt   string `json:"accessJwt"`
	RefreshJwt  string `json:"refreshJwt"`
	Handle      string `json:"handle"`
	DID         string `json:"did"`
	DisplayName string `json:"displayName,omitempty"`
}

// RefreshSession exchanges a refresh JWT for a new access/refresh token pair.
func (b *Bluesky) RefreshSession(pdsURL, refreshJwt string) (*BlueskySessionResponse, error) {
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

	req, err := http.NewRequest(http.MethodPost,
		pdsURL+"/xrpc/com.atproto.server.refreshSession", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+refreshJwt)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to refresh session: %s - %s", resp.Status, string(respBody))
	}

	var session BlueskySessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	return &session, nil
}

// CreateSession authenticates with an app password and returns session tokens.
func (b *Bluesky) CreateSession(pdsURL, identifier, appPassword string) (*BlueskySessionResponse, error) {
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

	body, err := json.Marshal(blueskySessionRequest{
		Identifier: identifier,
		Password:   appPassword,
	})
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Post(
		pdsURL+"/xrpc/com.atproto.server.createSession",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create session: %s - %s", resp.Status, string(respBody))
	}

	var session BlueskySessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session response: %w", err)
	}

	return &session, nil
}

// --- Post ---

type blueskyCreateRecordRequest struct {
	Repo       string          `json:"repo"`
	Collection string          `json:"collection"`
	Record     json.RawMessage `json:"record"`
}

type blueskyPostRecord struct {
	Type      string         `json:"$type"`
	Text      string         `json:"text"`
	CreatedAt string         `json:"createdAt"`
	Facets    []blueskyFacet `json:"facets,omitempty"`
	Embed     any            `json:"embed,omitempty"`
}

type blueskyEmbed struct {
	Type   string          `json:"$type"`
	Images []blueskyImage  `json:"images"`
}

type blueskyImage struct {
	Image blueskyBlob `json:"image"`
	Alt   string      `json:"alt"`
}

type blueskyBlob struct {
	Type     string     `json:"$type"`
	Ref      blobRef    `json:"ref"`
	MimeType string     `json:"mimeType"`
	Size     int64      `json:"size"`
}

type blobRef struct {
	Link string `json:"$link"`
}

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

// Facets for rich text (links and hashtags).
type blueskyFacet struct {
	Index    blueskyByteSlice  `json:"index"`
	Features []blueskyFeature  `json:"features"`
}

type blueskyByteSlice struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

type blueskyFeature struct {
	Type string `json:"$type"`
	// For links:
	URI string `json:"uri,omitempty"`
	// For hashtags:
	Tag string `json:"tag,omitempty"`
}

type blueskyCreateRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// parseFacets scans text for URLs and hashtags and returns the corresponding
// facet annotations using byte offsets (as required by the AT Protocol).
func parseFacets(text string) []blueskyFacet {
	textBytes := []byte(text)
	var facets []blueskyFacet

	for _, loc := range urlRe.FindAllIndex(textBytes, -1) {
		uri := string(textBytes[loc[0]:loc[1]])
		facets = append(facets, blueskyFacet{
			Index: blueskyByteSlice{ByteStart: loc[0], ByteEnd: loc[1]},
			Features: []blueskyFeature{
				{Type: "app.bsky.richtext.facet#link", URI: uri},
			},
		})
	}

	for _, loc := range hashtagRe.FindAllIndex(textBytes, -1) {
		tag := string(textBytes[loc[0]+1 : loc[1]]) // strip leading '#'
		facets = append(facets, blueskyFacet{
			Index: blueskyByteSlice{ByteStart: loc[0], ByteEnd: loc[1]},
			Features: []blueskyFeature{
				{Type: "app.bsky.richtext.facet#tag", Tag: tag},
			},
		})
	}

	sort.Slice(facets, func(i, j int) bool {
		return facets[i].Index.ByteStart < facets[j].Index.ByteStart
	})

	return facets
}

func (b *Bluesky) Post(ctx context.Context, account *models.Account, content string, mediaIDs []string) (*models.PostResult, error) {
	pdsURL := account.InstanceURL
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

	record := blueskyPostRecord{
		Type:      "app.bsky.feed.post",
		Text:      content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Facets:    parseFacets(content),
	}

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

	recordJSON, err := json.Marshal(record)
	if err != nil {
		return failResult(account, "failed to marshal post record"), nil
	}

	// DID is stored in ChannelID (see handleBlueskyConnect).
	reqBody, err := json.Marshal(blueskyCreateRecordRequest{
		Repo:       account.ChannelID,
		Collection: "app.bsky.feed.post",
		Record:     recordJSON,
	})
	if err != nil {
		return failResult(account, "failed to marshal create record request"), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		pdsURL+"/xrpc/com.atproto.repo.createRecord", bytes.NewReader(reqBody))
	if err != nil {
		return failResult(account, err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return failResult(account, err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return failResult(account, fmt.Sprintf("%s - %s", resp.Status, string(respBody))), nil
	}

	var createResp blueskyCreateRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return failResult(account, "failed to parse response"), nil
	}

	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     true,
		PostURL:     atURIToURL(createResp.URI, account.Username),
	}, nil
}

// --- Media upload ---

type blueskyUploadBlobResponse struct {
	Blob blueskyBlob `json:"blob"`
}

// UploadMedia uploads a file to the Bluesky PDS and returns a JSON-encoded
// blob reference that Post() embeds directly in the record.
func (b *Bluesky) UploadMedia(ctx context.Context, account *models.Account, file io.Reader, filename, contentType string) (string, error) {
	pdsURL := account.InstanceURL
	if pdsURL == "" {
		pdsURL = blueskyDefaultPDS
	}

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

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to upload blob: %s - %s", resp.Status, string(respBody))
	}

	var uploadResp blueskyUploadBlobResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	// Return the blob as JSON so Post() can reconstruct it for the embed.
	blobJSON, err := json.Marshal(uploadResp.Blob)
	if err != nil {
		return "", fmt.Errorf("failed to marshal blob: %w", err)
	}

	return string(blobJSON), nil
}

// --- Helpers ---

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

// atURIToURL converts an AT URI (at://did/app.bsky.feed.post/rkey) to a
// bsky.app web URL using the account handle.
func atURIToURL(uri, handle string) string {
	// at://did:plc:xxx/app.bsky.feed.post/rkey
	parts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if len(parts) != 3 {
		return uri
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handle, parts[2])
}

func failResult(account *models.Account, errMsg string) *models.PostResult {
	return &models.PostResult{
		AccountID:   account.ID,
		Provider:    string(account.Provider),
		DisplayName: account.DisplayName,
		Success:     false,
		Error:       errMsg,
	}
}
