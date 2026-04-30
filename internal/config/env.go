package config

import (
	"log/slog"
	"os"
)

const (
	braveAPIKeyEnv           = "BRAVE_API_KEY"           //nolint:gosec // Environment variable name, not credential value.
	youTubeAPIKeyEnv         = "YOUTUBE_API_KEY"         //nolint:gosec // Environment variable name, not credential value.
	podcastIndexAPIKeyEnv    = "PODCASTINDEX_API_KEY"    //nolint:gosec // Environment variable name, not credential value.
	podcastIndexAPISecretEnv = "PODCASTINDEX_API_SECRET" //nolint:gosec // Environment variable name, not credential value.
	xBearerTokenEnv          = "X_BEARER_TOKEN"          //nolint:gosec // Environment variable name, not credential value.
	openAIAPIKeyEnv          = "OPENAI_API_KEY"          //nolint:gosec // Environment variable name, not credential value.
	redditClientIDEnv        = "REDDIT_CLIENT_ID"
	redditClientSecretEnv    = "REDDIT_CLIENT_SECRET" //nolint:gosec // Environment variable name, not credential value.
	redditUserAgentEnv       = "REDDIT_USER_AGENT"
	rodBrowserBinEnv         = "ROD_BROWSER_BIN"
)

// LoadFromEnv reads settings from environment variables and logs only presence flags.
func LoadFromEnv(logger *slog.Logger) (Settings, error) {
	settings := Settings{
		BraveAPIKey:           os.Getenv(braveAPIKeyEnv),
		YouTubeAPIKey:         os.Getenv(youTubeAPIKeyEnv),
		PodcastIndexAPIKey:    os.Getenv(podcastIndexAPIKeyEnv),
		PodcastIndexAPISecret: os.Getenv(podcastIndexAPISecretEnv),
		XBearerToken:          os.Getenv(xBearerTokenEnv),
		OpenAIAPIKey:          os.Getenv(openAIAPIKeyEnv),
		RedditClientID:        os.Getenv(redditClientIDEnv),
		RedditClientSecret:    os.Getenv(redditClientSecretEnv),
		RedditUserAgent:       os.Getenv(redditUserAgentEnv),
		RodBrowserBin:         os.Getenv(rodBrowserBinEnv),
	}

	logSettingsPresence(logger, settings)

	return settings, nil
}

func logSettingsPresence(logger *slog.Logger, settings Settings) {
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
}
