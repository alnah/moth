package podcast

type podcastSearchResponse struct {
	Count int           `json:"count"`
	Feeds []podcastFeed `json:"feeds"`
}

type podcastFeed struct {
	ID           int               `json:"id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	URL          string            `json:"url"`
	Link         string            `json:"link"`
	Author       string            `json:"author"`
	Image        string            `json:"image"`
	EpisodeCount int               `json:"episodeCount"`
	Categories   map[string]string `json:"categories"`
}

type podcastEpisodesResponse struct {
	Count int              `json:"count"`
	Items []podcastEpisode `json:"items"`
}

type podcastEpisode struct {
	ID              int    `json:"id"`
	FeedID          int    `json:"feedId"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Link            string `json:"link"`
	GUID            string `json:"guid"`
	DatePublished   int    `json:"datePublished"`
	Duration        int    `json:"duration"`
	EnclosureURL    string `json:"enclosureUrl"`
	EnclosureType   string `json:"enclosureType"`
	EnclosureLength int    `json:"enclosureLength"`
}
