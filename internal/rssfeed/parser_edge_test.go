package rssfeed

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseHonorsCanceledContextBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewParser().Parse(ctx, strings.NewReader(`<rss></rss>`), ParseOptions{})
	if err == nil {
		t.Fatal("Parse(canceled context) error = nil, want context canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Parse(canceled context) error = %v, want context.Canceled", err)
	}
}

func TestParseRejectsFeedOverMaxBytes(t *testing.T) {
	reader := strings.NewReader(`{"title":"too large"}`)
	_, err := NewParser().Parse(context.Background(), reader, ParseOptions{MaxBytes: 4})
	if err == nil {
		t.Fatal("Parse(feed over max bytes) error = nil, want size error")
	}
}

func TestParseMalformedJSONFeedReturnsSemanticError(t *testing.T) {
	_, err := NewParser().Parse(context.Background(), strings.NewReader(`{"items":`), ParseOptions{})
	if err == nil {
		t.Fatal("Parse(malformed JSON feed) error = nil, want malformed feed error")
	}
	if !errors.Is(err, ErrMalformedFeed) {
		t.Fatalf("Parse(malformed JSON feed) error = %v, want ErrMalformedFeed", err)
	}
}

func TestParseJSONFeedWithoutAttachmentsMapsEmptyEnclosures(t *testing.T) {
	const feedJSON = `{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "JSON Notes",
  "feed_url": "https://example.test/feed.json",
  "items": [{"id":"note-1","content_text":"short note"}]
}`

	feed, err := NewParser().Parse(context.Background(), strings.NewReader(feedJSON), ParseOptions{})
	if err != nil {
		t.Fatalf("Parse(JSON feed without attachments) error = %v, want nil", err)
	}
	if feed.URL != "https://example.test/feed.json" {
		t.Fatalf("feed URL = %q, want JSON feed_url", feed.URL)
	}
	if len(feed.Items) != 1 || len(feed.Items[0].Enclosures) != 0 {
		t.Fatalf("feed items = %#v, want one item without enclosures", feed.Items)
	}
}
