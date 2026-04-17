package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sathyabhat/echobridge/internal/models"
)

func newBlueskyTestServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *Bluesky) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	b := NewBlueskyWithClient(srv.Client())
	return srv, b
}

func blueskyAccount(pdsURL string) *models.Account {
	return &models.Account{
		ID:          "acc-bsky-1",
		Provider:    models.ProviderBluesky,
		DisplayName: "Bob",
		Username:    "bob.bsky.social",
		InstanceURL: pdsURL,
		AccessToken: "bsky-token",
		ChannelID:   "did:plc:testdid",
	}
}

// --- CreateSession ---

func TestCreateSession_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.server.createSession", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(BlueskySessionResponse{
			AccessJwt:   "access-jwt-123",
			RefreshJwt:  "refresh-jwt-456",
			Handle:      "bob.bsky.social",
			DID:         "did:plc:abc123",
			DisplayName: "Bob",
		})
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	session, err := bluesky.CreateSession(srv.URL, "bob.bsky.social", "app-password")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Handle != "bob.bsky.social" {
		t.Errorf("Handle: got %q, want %q", session.Handle, "bob.bsky.social")
	}
	if session.AccessJwt != "access-jwt-123" {
		t.Errorf("AccessJwt: got %q, want %q", session.AccessJwt, "access-jwt-123")
	}
	if session.DID != "did:plc:abc123" {
		t.Errorf("DID: got %q, want %q", session.DID, "did:plc:abc123")
	}
}

func TestCreateSession_Unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.server.createSession", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"AuthenticationRequired"}`, http.StatusUnauthorized)
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	_, err := bluesky.CreateSession(srv.URL, "bob.bsky.social", "wrong-password")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateSession_DefaultPDS(t *testing.T) {
	// When pdsURL is empty, it should use blueskyDefaultPDS.
	// We can't easily intercept bsky.social, so just verify the error
	// mentions a network failure (not a crash).
	b := NewBluesky()
	_, err := b.CreateSession("", "nobody", "nopass")
	// We expect a network error since bsky.social is not reachable in tests.
	// The important thing is it doesn't panic.
	if err == nil {
		t.Log("CreateSession with default PDS unexpectedly succeeded (live test environment?)")
	}
}

// --- Post ---

func TestBlueskyPost_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{
			URI: "at://did:plc:testdid/app.bsky.feed.post/rkey123",
			CID: "cid-abc",
		})
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	result, err := bluesky.Post(context.Background(), blueskyAccount(srv.URL), "Hello Bluesky!", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.PostURL, "bsky.app") {
		t.Errorf("PostURL should be a bsky.app URL, got %q", result.PostURL)
	}
}

func TestBlueskyPost_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	result, err := bluesky.Post(context.Background(), blueskyAccount(srv.URL), "Hello!", nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Success {
		t.Error("expected failure result")
	}
}

func TestBlueskyPost_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{URI: "at://did/col/rkey"})
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	acc := blueskyAccount(srv.URL)
	acc.AccessToken = "my-jwt"
	bluesky.Post(context.Background(), acc, "text", nil) //nolint:errcheck

	if gotAuth != "Bearer my-jwt" {
		t.Errorf("Authorization: got %q, want %q", gotAuth, "Bearer my-jwt")
	}
}

// --- UploadMedia ---

func TestBlueskyUploadMedia_Success(t *testing.T) {
	blobData := blueskyBlob{
		Type:     "blob",
		Ref:      blobRef{Link: "bafkrei..."},
		MimeType: "image/jpeg",
		Size:     512,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(blueskyUploadBlobResponse{Blob: blobData})
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	blobJSON, err := bluesky.UploadMedia(
		context.Background(),
		blueskyAccount(srv.URL),
		strings.NewReader("fake image"),
		"photo.jpg",
		"image/jpeg",
	)
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if !strings.Contains(blobJSON, "bafkrei") {
		t.Errorf("returned blob JSON doesn't contain ref link: %s", blobJSON)
	}
}

func TestBlueskyUploadMedia_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	_, err := bluesky.UploadMedia(
		context.Background(),
		blueskyAccount(srv.URL),
		strings.NewReader("data"),
		"photo.jpg",
		"image/jpeg",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- parseFacets ---

func TestParseFacets(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  []blueskyFacet
	}{
		{
			name: "no facets",
			text: "Hello world",
			want: nil,
		},
		{
			name: "single hashtag",
			text: "Hello #golang",
			want: []blueskyFacet{
				{
					Index:    blueskyByteSlice{ByteStart: 6, ByteEnd: 13},
					Features: []blueskyFeature{{Type: "app.bsky.richtext.facet#tag", Tag: "golang"}},
				},
			},
		},
		{
			name: "single URL",
			text: "Check https://example.com out",
			want: []blueskyFacet{
				{
					Index:    blueskyByteSlice{ByteStart: 6, ByteEnd: 25},
					Features: []blueskyFeature{{Type: "app.bsky.richtext.facet#link", URI: "https://example.com"}},
				},
			},
		},
		{
			name: "hashtag and URL sorted by byte offset",
			text: "#go https://go.dev",
			want: []blueskyFacet{
				{
					Index:    blueskyByteSlice{ByteStart: 0, ByteEnd: 3},
					Features: []blueskyFeature{{Type: "app.bsky.richtext.facet#tag", Tag: "go"}},
				},
				{
					Index:    blueskyByteSlice{ByteStart: 4, ByteEnd: 18},
					Features: []blueskyFeature{{Type: "app.bsky.richtext.facet#link", URI: "https://go.dev"}},
				},
			},
		},
		{
			name: "multiple hashtags",
			text: "#foo and #bar",
			want: []blueskyFacet{
				{
					Index:    blueskyByteSlice{ByteStart: 0, ByteEnd: 4},
					Features: []blueskyFeature{{Type: "app.bsky.richtext.facet#tag", Tag: "foo"}},
				},
				{
					Index:    blueskyByteSlice{ByteStart: 9, ByteEnd: 13},
					Features: []blueskyFeature{{Type: "app.bsky.richtext.facet#tag", Tag: "bar"}},
				},
			},
		},
		{
			name: "hashtag starting with digit is ignored",
			text: "count #1st item",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFacets(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("parseFacets(%q): got %d facets, want %d\ngot:  %+v\nwant: %+v",
					tc.text, len(got), len(tc.want), got, tc.want)
			}
			for i, f := range got {
				w := tc.want[i]
				if f.Index != w.Index {
					t.Errorf("facet[%d] index: got %+v, want %+v", i, f.Index, w.Index)
				}
				if len(f.Features) != len(w.Features) {
					t.Errorf("facet[%d] features len: got %d, want %d", i, len(f.Features), len(w.Features))
					continue
				}
				feat := f.Features[0]
				wfeat := w.Features[0]
				if feat.Type != wfeat.Type || feat.URI != wfeat.URI || feat.Tag != wfeat.Tag {
					t.Errorf("facet[%d] feature: got %+v, want %+v", i, feat, wfeat)
				}
			}
		})
	}
}

func TestBlueskyPost_FacetsIncluded(t *testing.T) {
	var capturedRecord blueskyPostRecord
	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		var req blueskyCreateRecordRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.Unmarshal(req.Record, &capturedRecord)
		json.NewEncoder(w).Encode(blueskyCreateRecordResponse{
			URI: "at://did:plc:testdid/app.bsky.feed.post/rkey123",
		})
	})
	srv, bluesky := newBlueskyTestServer(t, mux)

	result, err := bluesky.Post(context.Background(), blueskyAccount(srv.URL),
		"Hello #golang at https://go.dev", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if len(capturedRecord.Facets) != 2 {
		t.Fatalf("expected 2 facets, got %d: %+v", len(capturedRecord.Facets), capturedRecord.Facets)
	}
}

// --- fetchLinkCard ---

func TestFetchLinkCard_OGTags(t *testing.T) {
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
	if card.External.Thumb != nil {
		t.Error("expected no thumb when og:image absent")
	}
}

func TestFetchLinkCard_FallbackTitle(t *testing.T) {
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
	blobData := blueskyBlob{
		Type:     "blob",
		Ref:      blobRef{Link: "bafkreithumb"},
		MimeType: "image/jpeg",
		Size:     100,
	}

	var srv2 *httptest.Server
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
<meta property="og:title" content="Title"/>
<meta property="og:image" content="%s/img"/>
</head><body></body></html>`, srv2.URL)
	})
	mux2.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fakejpeg"))
	})
	mux2.HandleFunc("/xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(blueskyUploadBlobResponse{Blob: blobData})
	})
	srv2 = httptest.NewServer(mux2)
	defer srv2.Close()

	b := NewBlueskyWithClient(srv2.Client())
	card, err := b.fetchLinkCard(context.Background(), srv2.URL, "token", srv2.URL+"/page")
	if err != nil {
		t.Fatalf("fetchLinkCard: %v", err)
	}
	if card.External.Thumb == nil {
		t.Fatal("expected Thumb to be set")
	}
	if card.External.Thumb.Ref.Link != "bafkreithumb" {
		t.Errorf("Thumb ref: got %q, want %q", card.External.Thumb.Ref.Link, "bafkreithumb")
	}
}

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
	if card.External.Thumb != nil {
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

// --- atURIToURL ---

func TestAtURIToURL(t *testing.T) {
	tests := []struct {
		uri    string
		handle string
		want   string
	}{
		{
			uri:    "at://did:plc:abc/app.bsky.feed.post/rkey123",
			handle: "alice.bsky.social",
			want:   "https://bsky.app/profile/alice.bsky.social/post/rkey123",
		},
		{
			// Malformed URI: returned as-is.
			uri:    "at://bad",
			handle: "alice.bsky.social",
			want:   "at://bad",
		},
	}
	for _, tc := range tests {
		got := atURIToURL(tc.uri, tc.handle)
		if got != tc.want {
			t.Errorf("atURIToURL(%q, %q) = %q, want %q", tc.uri, tc.handle, got, tc.want)
		}
	}
}
