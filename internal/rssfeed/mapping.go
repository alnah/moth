package rssfeed

import (
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	gofeedjson "github.com/mmcdole/gofeed/json"
)

func mapUniversalFeed(parsed *gofeed.Feed, feedURL string) Feed {
	feed := Feed{
		Title: parsed.Title,
		URL:   firstNonEmpty(feedURL, parsed.FeedLink),
		Items: make([]Item, 0, len(parsed.Items)),
	}
	for _, parsedItem := range parsed.Items {
		feed.Items = append(feed.Items, mapUniversalItem(parsedItem))
	}

	return feed
}

func mapUniversalItem(parsed *gofeed.Item) Item {
	item := Item{
		Title:      parsed.Title,
		Link:       firstItemLink(parsed),
		GUID:       parsed.GUID,
		Enclosures: make([]Enclosure, 0, len(parsed.Enclosures)),
	}
	if parsed.ITunesExt != nil {
		item.Duration = parseDuration(parsed.ITunesExt.Duration)
	}
	for _, enclosure := range parsed.Enclosures {
		item.Enclosures = append(item.Enclosures, Enclosure{
			URL:    enclosure.URL,
			Type:   enclosure.Type,
			Length: parseInt64(enclosure.Length),
		})
	}

	return item
}

func mapJSONFeed(parsed *gofeedjson.Feed, feedURL string) Feed {
	feed := Feed{
		Title: parsed.Title,
		URL:   firstNonEmpty(feedURL, parsed.FeedURL),
		Items: make([]Item, 0, len(parsed.Items)),
	}
	for _, parsedItem := range parsed.Items {
		feed.Items = append(feed.Items, mapJSONItem(parsedItem))
	}

	return feed
}

func mapJSONItem(parsed *gofeedjson.Item) Item {
	item := Item{
		Title:      firstNonEmpty(parsed.Title, parsed.ID),
		Link:       firstNonEmpty(parsed.URL, parsed.ExternalURL),
		GUID:       parsed.ID,
		Enclosures: []Enclosure{},
	}
	if parsed.Attachments == nil {
		return item
	}
	for _, attachment := range *parsed.Attachments {
		if item.Duration == 0 && attachment.DurationInSeconds > 0 {
			item.Duration = time.Duration(attachment.DurationInSeconds) * time.Second
		}
		item.Enclosures = append(item.Enclosures, Enclosure{
			URL:    attachment.URL,
			Type:   attachment.MimeType,
			Length: attachment.SizeInBytes,
		})
	}

	return item
}

func firstItemLink(item *gofeed.Item) string {
	if item.Link != "" {
		return item.Link
	}
	if len(item.Links) > 0 {
		return item.Links[0]
	}

	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func parseDuration(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	parts := strings.Split(raw, ":")
	seconds := int64(0)
	for _, part := range parts {
		value := parseInt64(part)
		seconds = seconds*60 + value
	}

	return time.Duration(seconds) * time.Second
}

func parseInt64(raw string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}

	return value
}
