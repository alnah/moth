package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alnah/moth/internal/content"
)

func TestSearchVideosSendsDocumentedRequestAndMapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/videos/search", searchRequest{
			query:      "go conference",
			count:      "5",
			country:    "US",
			language:   "en",
			safeSearch: "off",
			offset:     "10",
		})
		writeJSONResponse(t, w, `{
			"type": "videos",
			"results": [
				{
					"type": "video_result",
					"title": "GopherCon talk",
					"url": "https://video.example.com/watch/go",
					"description": "A practical Go conference talk.",
					"thumbnail": {"src": "https://video.example.com/thumb.jpg"},
					"duration": "PT3M20S",
					"publisher": "GopherCon"
				}
			]
		}`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

	result, err := client.SearchVideos(context.Background(), Options{
		Query:      "go conference",
		Count:      5,
		Country:    "US",
		Language:   "en",
		SafeSearch: "off",
		Offset:     10,
	})
	if err != nil {
		t.Fatalf("SearchVideos error = %v, want nil", err)
	}

	assertContentPack(t, result, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindVideo,
				URL:   "https://video.example.com/watch/go",
				Title: "GopherCon talk",
				Text:  "A practical Go conference talk.",
				Metadata: map[string]any{
					"thumbnail_url": "https://video.example.com/thumb.jpg",
					"duration":      "PT3M20S",
					"publisher":     "GopherCon",
				},
			},
		},
	})
	assertContentPackJSONWarningsAreArrays(t, result)
}

func TestSearchVideosOmitsEmptyOptionalMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/videos/search", searchRequest{
			query:  "minimal video",
			offset: "0",
		})
		writeJSONResponse(t, w, `{
			"type": "videos",
			"results": [
				{
					"type": "video_result",
					"title": "Minimal video",
					"url": "https://video.example.com/minimal",
					"description": "A video result without optional media fields.",
					"thumbnail": {}
				}
			]
		}`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

	result, err := client.SearchVideos(context.Background(), Options{Query: "minimal video"})
	if err != nil {
		t.Fatalf("SearchVideos error = %v, want nil", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("video items len = %d, want 1", len(result.Items))
	}

	item := result.Items[0]
	if item.Kind != content.KindVideo {
		t.Fatalf("video kind = %q, want %q", item.Kind, content.KindVideo)
	}
	if item.URL != "https://video.example.com/minimal" {
		t.Fatalf("video URL = %q, want mapped URL", item.URL)
	}
	assertNoMetadata(t, item.Metadata)
}
