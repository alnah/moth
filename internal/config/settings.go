package config

import (
	"log/slog"
	"os"
)

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

func LoadFromEnv(logger *slog.Logger) (Settings, error) {
	settings := Settings{
		BraveAPIKey:           os.Getenv("BRAVE_API_KEY"),
		YouTubeAPIKey:         os.Getenv("YOUTUBE_API_KEY"),
		PodcastIndexAPIKey:    os.Getenv("PODCASTINDEX_API_KEY"),
		PodcastIndexAPISecret: os.Getenv("PODCASTINDEX_API_SECRET"),
		XBearerToken:          os.Getenv("X_BEARER_TOKEN"),
		OpenAIAPIKey:          os.Getenv("OPENAI_API_KEY"),
		RedditClientID:        os.Getenv("REDDIT_CLIENT_ID"),
		RedditClientSecret:    os.Getenv("REDDIT_CLIENT_SECRET"),
		RedditUserAgent:       os.Getenv("REDDIT_USER_AGENT"),
		RodBrowserBin:         os.Getenv("ROD_BROWSER_BIN"),
	}

	if logger != nil {
		logger.Debug("loaded settings from environment",
			"brave_api_key_set", settings.BraveAPIKey != "",
			"youtube_api_key_set", settings.YouTubeAPIKey != "",
			"podcastindex_api_key_set", settings.PodcastIndexAPIKey != "",
			"podcastindex_api_secret_set", settings.PodcastIndexAPISecret != "",
			"x_bearer_token_set", settings.XBearerToken != "",
			"openai_api_key_set", settings.OpenAIAPIKey != "",
			"reddit_client_id_set", settings.RedditClientID != "",
			"reddit_client_secret_set", settings.RedditClientSecret != "",
			"reddit_user_agent_set", settings.RedditUserAgent != "",
			"rod_browser_bin_set", settings.RodBrowserBin != "",
		)
	}

	return settings, nil
}
