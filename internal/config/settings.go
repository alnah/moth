package config

// Settings contains optional credentials and paths loaded from the environment.
type Settings struct {
	BraveAPIKey           string
	YouTubeAPIKey         string
	PodcastIndexAPIKey    string
	PodcastIndexAPISecret string
	XBearerToken          string
	OpenAIAPIKey          string
	RedditClientID        string
	RedditClientSecret    string
	RedditUserAgent       string
	RodBrowserBin         string
}
