package config

// EnvironmentSettings contains non-secret settings loaded from environment variables.
type EnvironmentSettings struct {
	RodBrowserBin string
}

// Credentials contains provider credentials loaded from environment variables only.
type Credentials struct {
	BraveAPIKey           string
	YouTubeAPIKey         string
	PodcastIndexAPIKey    string
	PodcastIndexAPISecret string
	XBearerToken          string
	OpenAIAPIKey          string
	RedditClientID        string
	RedditClientSecret    string
	RedditUserAgent       string
}
