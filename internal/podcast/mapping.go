package podcast

import (
	"sort"

	"github.com/alnah/moth/internal/content"
)

func mapPodcastFeeds(feeds []podcastFeed) []content.Item {
	items := make([]content.Item, 0, len(feeds))
	for _, feed := range feeds {
		if feed.ID == 0 {
			continue
		}
		items = append(items, content.Item{
			Kind:  content.KindPodcast,
			URL:   feed.URL,
			Title: feed.Title,
			Text:  feed.Description,
			Metadata: map[string]any{
				"feed_id":       feed.ID,
				"site_url":      feed.Link,
				"author":        feed.Author,
				"image_url":     feed.Image,
				"episode_count": feed.EpisodeCount,
				"categories":    sortedCategoryNames(feed.Categories),
			},
		})
	}

	return items
}

func mapPodcastEpisodes(episodes []podcastEpisode) []content.Item {
	items := make([]content.Item, 0, len(episodes))
	for _, episode := range episodes {
		items = append(items, content.Item{
			Kind:  content.KindAudio,
			URL:   episode.EnclosureURL,
			Title: episode.Title,
			Text:  episode.Description,
			Metadata: map[string]any{
				"episode_id":        episode.ID,
				"feed_id":           episode.FeedID,
				"episode_url":       episode.Link,
				"guid":              episode.GUID,
				"published_at_unix": episode.DatePublished,
				"duration_seconds":  episode.Duration,
				"enclosure_type":    episode.EnclosureType,
				"enclosure_length":  episode.EnclosureLength,
			},
		})
	}

	return items
}

func sortedCategoryNames(categories map[string]string) []string {
	keys := make([]string, 0, len(categories))
	for key := range categories {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, categories[key])
	}

	return names
}
