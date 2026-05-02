package brave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alnah/moth/internal/content"
)

func TestSearchImagesSendsDocumentedRequestAndMapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/images/search", searchRequest{
			query:      "blue gopher",
			count:      "3",
			country:    "CA",
			language:   "en",
			safeSearch: "moderate",
			offset:     "6",
		})
		writeJSONResponse(t, w, `{
			"type": "images",
			"results": [
				{
					"type": "image_result",
					"title": "Blue gopher",
					"url": "https://example.com/gopher-page",
					"description": "A blue gopher mascot.",
					"thumbnail": {"src": "https://cdn.example.com/gopher-thumb.jpg"},
					"properties": {
						"url": "https://cdn.example.com/gopher-full.jpg",
						"width": 640,
						"height": 480
					}
				}
			]
		}`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

	result, err := client.SearchImages(context.Background(), SearchOptions{
		Query:      "blue gopher",
		Count:      3,
		Country:    "CA",
		Language:   "en",
		SafeSearch: "moderate",
		Offset:     6,
	})
	if err != nil {
		t.Fatalf("SearchImages error = %v, want nil", err)
	}

	assertContentPack(t, result, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindImage,
				URL:   "https://cdn.example.com/gopher-full.jpg",
				Title: "Blue gopher",
				Text:  "A blue gopher mascot.",
				Metadata: map[string]any{
					"page_url":      "https://example.com/gopher-page",
					"thumbnail_url": "https://cdn.example.com/gopher-thumb.jpg",
					"width":         640,
					"height":        480,
				},
			},
		},
	})
	assertContentPackJSONWarningsAreArrays(t, result)
}

func TestSearchImagesOmitsEmptyOptionalMediaMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/images/search", searchRequest{
			query:  "minimal image",
			offset: "0",
		})
		writeJSONResponse(t, w, `{
			"type": "images",
			"results": [
				{
					"type": "image_result",
					"title": "Minimal image",
					"url": "https://example.com/image-page",
					"description": "An image result without optional media fields.",
					"thumbnail": {},
					"properties": {}
				}
			]
		}`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

	result, err := client.SearchImages(context.Background(), SearchOptions{Query: "minimal image"})
	if err != nil {
		t.Fatalf("SearchImages error = %v, want nil", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("image items len = %d, want 1", len(result.Items))
	}

	item := result.Items[0]
	if item.Kind != content.KindImage {
		t.Fatalf("image kind = %q, want %q", item.Kind, content.KindImage)
	}
	if item.URL != "" {
		t.Fatalf("image URL = %q, want empty when properties.url is absent", item.URL)
	}
	assertMetadataString(t, item.Metadata, "page_url", "https://example.com/image-page")
	assertMetadataKeyMissing(t, item.Metadata, "thumbnail_url")
	assertMetadataKeyMissing(t, item.Metadata, "width")
	assertMetadataKeyMissing(t, item.Metadata, "height")
}
