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
	rodBrowserBinEnv         = "ROD_BROWSER_BIN"
)

// LoadFromEnv reads environment variables and logs only presence flags.
func LoadFromEnv(logger *slog.Logger) (Credentials, EnvironmentSettings, error) {
	credentials := Credentials{
		BraveAPIKey:           os.Getenv(braveAPIKeyEnv),
		YouTubeAPIKey:         os.Getenv(youTubeAPIKeyEnv),
		PodcastIndexAPIKey:    os.Getenv(podcastIndexAPIKeyEnv),
		PodcastIndexAPISecret: os.Getenv(podcastIndexAPISecretEnv),
		XBearerToken:          os.Getenv(xBearerTokenEnv),
		OpenAIAPIKey:          os.Getenv(openAIAPIKeyEnv),
	}
	settings := EnvironmentSettings{
		RodBrowserBin: os.Getenv(rodBrowserBinEnv),
	}

	logEnvironmentPresence(logger, credentials, settings)

	return credentials, settings, nil
}

func logEnvironmentPresence(logger *slog.Logger, credentials Credentials, settings EnvironmentSettings) {
	if logger != nil {
		logger.Debug("loaded settings from environment",
			"brave_api_key_set", credentials.BraveAPIKey != "",
			"youtube_api_key_set", credentials.YouTubeAPIKey != "",
			"podcastindex_api_key_set", credentials.PodcastIndexAPIKey != "",
			"podcastindex_api_secret_set", credentials.PodcastIndexAPISecret != "",
			"x_bearer_token_set", credentials.XBearerToken != "",
			"openai_api_key_set", credentials.OpenAIAPIKey != "",
			"rod_browser_bin_set", settings.RodBrowserBin != "",
		)
	}
}
