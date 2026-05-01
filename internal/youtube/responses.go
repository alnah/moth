package youtube

type youtubeSearchResponse struct {
	NextPageToken string                `json:"nextPageToken"`
	PageInfo      youtubePageInfo       `json:"pageInfo"`
	Items         []youtubeSearchResult `json:"items"`
}

type youtubePageInfo struct {
	TotalResults int `json:"totalResults"`
}

type youtubeSearchResult struct {
	ID      youtubeSearchID `json:"id"`
	Snippet youtubeSnippet  `json:"snippet"`
}

type youtubeSearchID struct {
	Kind    string `json:"kind"`
	VideoID string `json:"videoId"`
}

type youtubeVideosResponse struct {
	Items []youtubeVideo `json:"items"`
}

type youtubeVideo struct {
	ID             string                 `json:"id"`
	Snippet        youtubeSnippet         `json:"snippet"`
	ContentDetails youtubeContentDetails  `json:"contentDetails"`
	Statistics     youtubeVideoStatistics `json:"statistics"`
}

type youtubeSnippet struct {
	PublishedAt  string                      `json:"publishedAt"`
	ChannelID    string                      `json:"channelId"`
	Title        string                      `json:"title"`
	Description  string                      `json:"description"`
	Thumbnails   map[string]youtubeThumbnail `json:"thumbnails"`
	ChannelTitle string                      `json:"channelTitle"`
}

type youtubeThumbnail struct {
	URL string `json:"url"`
}

type youtubeContentDetails struct {
	Duration string `json:"duration"`
}

type youtubeVideoStatistics struct {
	ViewCount string `json:"viewCount"`
}
