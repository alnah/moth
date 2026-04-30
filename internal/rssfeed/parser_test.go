package rssfeed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseRSSMapsEnclosuresAndITunesDuration(t *testing.T) {
	const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Moth Radio</title>
    <item>
      <title>Episode 17</title>
      <link>https://podcasts.example/episodes/17</link>
      <guid>episode-17</guid>
      <itunes:duration>01:02:03</itunes:duration>
      <enclosure url="https://cdn.example/episode-17.mp3" type="audio/mpeg" length="123456" />
    </item>
  </channel>
</rss>`

	feed, err := NewParser().Parse(context.Background(), strings.NewReader(feedXML), ParseOptions{
		FeedURL: "https://podcasts.example/feed.xml",
	})
	if err != nil {
		t.Fatalf("Parse(RSS podcast feed) error = %v, want nil", err)
	}

	want := Feed{
		Title: "Moth Radio",
		URL:   "https://podcasts.example/feed.xml",
		Items: []Item{
			{
				Title:    "Episode 17",
				Link:     "https://podcasts.example/episodes/17",
				GUID:     "episode-17",
				Duration: time.Hour + 2*time.Minute + 3*time.Second,
				Enclosures: []Enclosure{
					{
						URL:    "https://cdn.example/episode-17.mp3",
						Type:   "audio/mpeg",
						Length: 123456,
					},
				},
			},
		},
	}
	assertFeed(t, feed, want)
}

func TestParseAtomMapsEnclosureLinks(t *testing.T) {
	const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Audio</title>
  <entry>
    <title>Atom Episode</title>
    <id>tag:podcasts.example,2026:atom-episode</id>
    <link rel="alternate" href="https://podcasts.example/atom-episode" />
    <link rel="enclosure" href="https://cdn.example/atom-episode.ogg" type="audio/ogg" length="3456" />
  </entry>
</feed>`

	feed, err := NewParser().Parse(context.Background(), strings.NewReader(feedXML), ParseOptions{})
	if err != nil {
		t.Fatalf("Parse(Atom feed) error = %v, want nil", err)
	}

	want := Feed{
		Title: "Atom Audio",
		Items: []Item{
			{
				Title: "Atom Episode",
				Link:  "https://podcasts.example/atom-episode",
				GUID:  "tag:podcasts.example,2026:atom-episode",
				Enclosures: []Enclosure{
					{URL: "https://cdn.example/atom-episode.ogg", Type: "audio/ogg", Length: 3456},
				},
			},
		},
	}
	assertFeed(t, feed, want)
}

func TestParseJSONFeedMapsAttachments(t *testing.T) {
	const feedJSON = `{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "JSON Audio",
  "items": [
    {
      "id": "json-episode",
      "title": "JSON Episode",
      "url": "https://podcasts.example/json-episode",
      "attachments": [
        {
          "url": "https://cdn.example/json-episode.m4a",
          "mime_type": "audio/mp4",
          "size_in_bytes": 7890,
          "duration_in_seconds": 125
        }
      ]
    }
  ]
}`

	feed, err := NewParser().Parse(context.Background(), strings.NewReader(feedJSON), ParseOptions{})
	if err != nil {
		t.Fatalf("Parse(JSON feed) error = %v, want nil", err)
	}

	want := Feed{
		Title: "JSON Audio",
		Items: []Item{
			{
				Title:    "JSON Episode",
				Link:     "https://podcasts.example/json-episode",
				GUID:     "json-episode",
				Duration: 125 * time.Second,
				Enclosures: []Enclosure{
					{URL: "https://cdn.example/json-episode.m4a", Type: "audio/mp4", Length: 7890},
				},
			},
		},
	}
	assertFeed(t, feed, want)
}

func TestParseMalformedFeedReturnsSemanticError(t *testing.T) {
	_, err := NewParser().Parse(context.Background(), strings.NewReader(`<rss><channel><item>`), ParseOptions{})
	if err == nil {
		t.Fatal("Parse(malformed feed) error = nil, want malformed feed error")
	}
	if !errors.Is(err, ErrMalformedFeed) {
		t.Fatalf("Parse(malformed feed) error = %v, want ErrMalformedFeed", err)
	}
}

func assertFeed(t *testing.T, got Feed, want Feed) {
	t.Helper()

	if got.Title != want.Title {
		t.Fatalf("feed title = %q, want %q", got.Title, want.Title)
	}
	if got.URL != want.URL {
		t.Fatalf("feed URL = %q, want %q", got.URL, want.URL)
	}
	if len(got.Items) != len(want.Items) {
		t.Fatalf("feed items len = %d, want %d", len(got.Items), len(want.Items))
	}
	for index := range want.Items {
		assertFeedItem(t, got.Items[index], want.Items[index])
	}
}

func assertFeedItem(t *testing.T, got Item, want Item) {
	t.Helper()

	if got.Title != want.Title {
		t.Fatalf("item title = %q, want %q", got.Title, want.Title)
	}
	if got.Link != want.Link {
		t.Fatalf("item link = %q, want %q", got.Link, want.Link)
	}
	if got.GUID != want.GUID {
		t.Fatalf("item GUID = %q, want %q", got.GUID, want.GUID)
	}
	if got.Duration != want.Duration {
		t.Fatalf("item duration = %s, want %s", got.Duration, want.Duration)
	}
	if len(got.Enclosures) != len(want.Enclosures) {
		t.Fatalf("item enclosures len = %d, want %d", len(got.Enclosures), len(want.Enclosures))
	}
	for index := range want.Enclosures {
		if got.Enclosures[index] != want.Enclosures[index] {
			t.Fatalf("item enclosure[%d] = %#v, want %#v", index, got.Enclosures[index], want.Enclosures[index])
		}
	}
}
