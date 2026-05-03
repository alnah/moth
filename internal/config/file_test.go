package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/limits"
)

func TestLoadFileAcceptsValidSchema(t *testing.T) {
	tests := []struct {
		name string
		json string
		want FileConfig
	}{
		{
			name: "all non-secret settings",
			json: `{
				"browser": {"bin": "/opt/moth/chrome"},
				"limits": {
					"timeout": "30s",
					"max_results": 10,
					"max_bytes": 26214400,
					"retries": 2,
					"retry_base": "500ms",
					"retry_max": "5s"
				}
			}`,
			want: FileConfig{
				Browser: BrowserConfig{Bin: "/opt/moth/chrome"},
				Limits: limits.Options{
					Timeout:    30 * time.Second,
					MaxResults: 10,
					MaxBytes:   26_214_400,
					Retries:    2,
					RetryBase:  500 * time.Millisecond,
					RetryMax:   5 * time.Second,
				},
				Presence: allConfigFieldsPresent(),
			},
		},
		{
			name: "empty root object",
			json: `{}`,
			want: FileConfig{},
		},
		{
			name: "empty nested objects",
			json: `{"browser": {}, "limits": {}}`,
			want: FileConfig{},
		},
		{
			name: "explicit zero limits",
			json: `{
				"limits": {
					"timeout": "0s",
					"max_results": 0,
					"max_bytes": 0,
					"retries": 0,
					"retry_base": "0s",
					"retry_max": "0s"
				}
			}`,
			want: FileConfig{Presence: FileConfigPresence{Limits: allLimitFieldsPresent()}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadFile(writeConfigFile(t, tt.json))
			if err != nil {
				t.Fatalf("LoadFile() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("LoadFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadFileRejectsInvalidSchema(t *testing.T) {
	tests := []struct {
		name             string
		json             string
		wantMessageParts []string
		wantUnsupported  bool
	}{
		{
			name:             "unknown top-level field",
			json:             `{"unknown": true}`,
			wantMessageParts: []string{"unsupported config field", "unknown"},
			wantUnsupported:  true,
		},
		{
			name:             "unknown browser field",
			json:             `{"browser": {"path": "/tmp/chrome"}}`,
			wantMessageParts: []string{"unsupported config field", "browser.path"},
			wantUnsupported:  true,
		},
		{
			name:             "unknown limits field",
			json:             `{"limits": {"count": 10}}`,
			wantMessageParts: []string{"unsupported config field", "limits.count"},
			wantUnsupported:  true,
		},
		{name: "malformed JSON", json: `{"limits":`, wantMessageParts: []string{"JSON"}},
		{name: "root null", json: `null`, wantMessageParts: []string{"root must be an object"}},
		{name: "browser null", json: `{"browser": null}`, wantMessageParts: []string{"browser", "must be an object"}},
		{name: "limits null", json: `{"limits": null}`, wantMessageParts: []string{"limits", "must be an object"}},
		{name: "empty browser bin", json: `{"browser": {"bin": ""}}`, wantMessageParts: []string{"browser.bin"}},
		{name: "non-string browser bin", json: `{"browser": {"bin": 7}}`, wantMessageParts: []string{"browser.bin"}},
		{name: "non-string timeout", json: `{"limits": {"timeout": 7}}`, wantMessageParts: []string{"timeout"}},
		{name: "invalid timeout", json: `{"limits": {"timeout": "soon"}}`, wantMessageParts: []string{"timeout"}},
		{
			name:             "negative timeout",
			json:             `{"limits": {"timeout": "-1s"}}`,
			wantMessageParts: []string{"timeout", "non-negative"},
		},
		{
			name:             "invalid retry_base",
			json:             `{"limits": {"retry_base": "eventually"}}`,
			wantMessageParts: []string{"retry_base"},
		},
		{name: "invalid retry_max", json: `{"limits": {"retry_max": "later"}}`, wantMessageParts: []string{"retry_max"}},
		{
			name:             "negative max_results",
			json:             `{"limits": {"max_results": -1}}`,
			wantMessageParts: []string{"max_results", "non-negative"},
		},
		{
			name:             "negative max_bytes",
			json:             `{"limits": {"max_bytes": -1}}`,
			wantMessageParts: []string{"max_bytes", "non-negative"},
		},
		{
			name:             "negative retries",
			json:             `{"limits": {"retries": -1}}`,
			wantMessageParts: []string{"retries", "non-negative"},
		},
		{
			name:             "non-numeric max_results",
			json:             `{"limits": {"max_results": "ten"}}`,
			wantMessageParts: []string{"max_results"},
		},
		{name: "non-numeric max_bytes", json: `{"limits": {"max_bytes": "big"}}`, wantMessageParts: []string{"max_bytes"}},
		{name: "non-numeric retries", json: `{"limits": {"retries": "many"}}`, wantMessageParts: []string{"retries"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFile(writeConfigFile(t, tt.json))
			if err == nil {
				t.Fatal("LoadFile() error = nil, want schema error")
			}
			if tt.wantUnsupported && !errors.Is(err, ErrUnsupportedConfigField) {
				t.Fatalf("LoadFile() error = %v, want ErrUnsupportedConfigField", err)
			}
			assertErrorContains(t, err, tt.wantMessageParts...)
		})
	}
}

func TestLoadFileRejectsSecretLookingFieldsWithoutEchoingValues(t *testing.T) {
	secretFields := []string{
		"brave_api_key",
		"youtube_api_key",
		"podcastindex_api_key",
		"podcastindex_api_secret",
		"x_bearer_token",
		"openai_api_key",
		"reddit_client_id",
		"reddit_client_secret",
		"api_key",
		"token",
		"secret",
		"password",
	}

	for _, field := range secretFields {
		t.Run(field, func(t *testing.T) {
			secretValue := "must-not-load-" + field
			_, err := LoadFile(writeConfigFile(t, fmt.Sprintf(`{"%s":%q}`, field, secretValue)))
			if !errors.Is(err, ErrUnsupportedConfigField) {
				t.Fatalf("LoadFile() error = %v, want ErrUnsupportedConfigField", err)
			}
			assertErrorContains(t, err, "unsupported config field", field)
			assertErrorOmits(t, err, secretValue)
		})
	}
}

func TestLoadFileDoesNotReadProviderCredentialsFromEnvironment(t *testing.T) {
	secrets := map[string]string{ //nolint:gosec // Test values verify config does not load credentials.
		"BRAVE_API_KEY":           "env-brave-secret",
		"YOUTUBE_API_KEY":         "env-youtube-secret",
		"PODCASTINDEX_API_KEY":    "env-podcast-key-secret",
		"PODCASTINDEX_API_SECRET": "env-podcast-secret-secret",
		"X_BEARER_TOKEN":          "env-x-token-secret",
		"OPENAI_API_KEY":          "env-openai-secret",
		"REDDIT_CLIENT_ID":        "env-reddit-client-id-secret",
		"REDDIT_CLIENT_SECRET":    "env-reddit-client-secret",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}

	settings, err := LoadFile(writeConfigFile(t, `{"limits": {"max_results": 3}}`))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal loaded config: %v", err)
	}
	for name, value := range secrets {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("LoadFile() included %s value %q in %s", name, value, encoded)
		}
	}
}

func allConfigFieldsPresent() FileConfigPresence {
	return FileConfigPresence{BrowserBin: true, Limits: allLimitFieldsPresent()}
}

func allLimitFieldsPresent() LimitsConfigPresence {
	return LimitsConfigPresence{
		Timeout:    true,
		MaxResults: true,
		MaxBytes:   true,
		Retries:    true,
		RetryBase:  true,
		RetryMax:   true,
	}
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want error containing %q", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("error = %v, want it to contain %q", err, part)
		}
	}
}

func assertErrorOmits(t *testing.T, err error, forbidden string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want error that omits %q", forbidden)
	}
	if strings.Contains(err.Error(), forbidden) {
		t.Fatalf("error = %v, want it to omit %q", err, forbidden)
	}
}

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "moth.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return path
}
