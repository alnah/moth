package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLoadFromEnvReadsProviderSecretsWithoutLoggingValues(t *testing.T) {
	secrets := map[string]string{
		"BRAVE_API_KEY":           "brave-secret",
		"YOUTUBE_API_KEY":         "youtube-secret",
		"PODCASTINDEX_API_KEY":    "podcast-key-secret",
		"PODCASTINDEX_API_SECRET": "podcast-secret-secret",
		"X_BEARER_TOKEN":          "x-token-secret",
		"OPENAI_API_KEY":          "openai-secret",
		"REDDIT_CLIENT_ID":        "reddit-client-id-secret",
		"REDDIT_CLIENT_SECRET":    "reddit-client-secret-secret",
		"REDDIT_USER_AGENT":       "moth-test by u/alnah",
		"ROD_BROWSER_BIN":         "/tmp/test-chromium",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	settings, err := LoadFromEnv(logger)
	if err != nil {
		t.Fatalf("load settings from env: %v", err)
	}

	got := map[string]string{
		"BRAVE_API_KEY":           settings.BraveAPIKey,
		"YOUTUBE_API_KEY":         settings.YouTubeAPIKey,
		"PODCASTINDEX_API_KEY":    settings.PodcastIndexAPIKey,
		"PODCASTINDEX_API_SECRET": settings.PodcastIndexAPISecret,
		"X_BEARER_TOKEN":          settings.XBearerToken,
		"OPENAI_API_KEY":          settings.OpenAIAPIKey,
		"REDDIT_CLIENT_ID":        settings.RedditClientID,
		"REDDIT_CLIENT_SECRET":    settings.RedditClientSecret,
		"REDDIT_USER_AGENT":       settings.RedditUserAgent,
		"ROD_BROWSER_BIN":         settings.RodBrowserBin,
	}
	for name, want := range secrets {
		if got[name] != want {
			t.Fatalf("%s = %q, want %q", name, got[name], want)
		}
	}

	logs := logOutput.String()
	for name, value := range secrets {
		if name == "REDDIT_USER_AGENT" || name == "ROD_BROWSER_BIN" {
			continue
		}
		if strings.Contains(logs, value) {
			t.Fatalf("logs expose %s value %q in %q", name, value, logs)
		}
	}
}
