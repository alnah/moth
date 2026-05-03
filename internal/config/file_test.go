package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFileLoadsSupportedJSONFields(t *testing.T) {
	path := writeConfigFile(t, `{
		"browser": {"bin": "/Applications/Chromium.app/Contents/MacOS/Chromium"},
		"limits": {
			"timeout": "30s",
			"max_results": 10,
			"max_bytes": 26214400,
			"retries": 2,
			"retry_base": "500ms",
			"retry_max": "5s"
		}
	}`)

	settings, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load config file: %v", err)
	}

	if settings.Browser.Bin != "/Applications/Chromium.app/Contents/MacOS/Chromium" {
		t.Fatalf("browser.bin = %q, want configured browser path", settings.Browser.Bin)
	}
	if settings.Limits.Timeout != 30*time.Second {
		t.Fatalf("limits.timeout = %v, want 30s", settings.Limits.Timeout)
	}
	if settings.Limits.MaxResults != 10 {
		t.Fatalf("limits.max_results = %d, want 10", settings.Limits.MaxResults)
	}
	if settings.Limits.MaxBytes != 26_214_400 {
		t.Fatalf("limits.max_bytes = %d, want 26214400", settings.Limits.MaxBytes)
	}
	if settings.Limits.Retries != 2 {
		t.Fatalf("limits.retries = %d, want 2", settings.Limits.Retries)
	}
	if settings.Limits.RetryBase != 500*time.Millisecond {
		t.Fatalf("limits.retry_base = %v, want 500ms", settings.Limits.RetryBase)
	}
	if settings.Limits.RetryMax != 5*time.Second {
		t.Fatalf("limits.retry_max = %v, want 5s", settings.Limits.RetryMax)
	}
}

func TestLoadFileEmptyJSONObjectYieldsZeroOverrides(t *testing.T) {
	settings, err := LoadFile(writeConfigFile(t, `{}`))
	if err != nil {
		t.Fatalf("load empty config object: %v", err)
	}

	if settings.Browser.Bin != "" {
		t.Fatalf("browser.bin = %q, want zero override", settings.Browser.Bin)
	}
	if settings.Limits.Timeout != 0 || settings.Limits.MaxResults != 0 || settings.Limits.MaxBytes != 0 ||
		settings.Limits.Retries != 0 || settings.Limits.RetryBase != 0 || settings.Limits.RetryMax != 0 {
		t.Fatalf("limits = %#v, want zero overrides", settings.Limits)
	}
}

func TestLoadFileRejectsUnknownFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
		json  string
	}{
		{name: "top level", field: "unknown", json: `{"unknown": true}`},
		{name: "browser", field: "path", json: `{"browser": {"path": "/tmp/chrome"}}`},
		{name: "limits", field: "count", json: `{"limits": {"count": 10}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFile(writeConfigFile(t, tt.json))
			assertUnsupportedConfigField(t, err, tt.field)
		})
	}
}

func TestLoadFileRejectsSecretLookingFields(t *testing.T) {
	secretFields := []string{
		"brave_api_key",
		"youtube_api_key",
		"podcastindex_api_key",
		"podcastindex_api_secret",
		"x_bearer_token",
		"openai_api_key",
		"api_key",
		"token",
		"secret",
		"password",
	}

	for _, field := range secretFields {
		t.Run(field, func(t *testing.T) {
			payload := fmt.Sprintf(`{"%s":"redacted-test-value"}`, field)
			_, err := LoadFile(writeConfigFile(t, payload))
			assertUnsupportedConfigField(t, err, field)
		})
	}
}

func TestLoadFileRejectsSecretLookingFieldWithoutEchoingValue(t *testing.T) {
	_, err := LoadFile(writeConfigFile(t, `{"openai_api_key":"must-not-load"}`))
	assertUnsupportedConfigField(t, err, "openai_api_key")
	assertErrorDoesNotContain(t, err, "must-not-load")
}

func TestLoadFileRejectsMalformedJSON(t *testing.T) {
	_, err := LoadFile(writeConfigFile(t, `{"limits":`))
	if err == nil {
		t.Fatal("load malformed JSON error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("error = %v, want JSON parse context", err)
	}
}

func TestLoadFileRejectsInvalidDurations(t *testing.T) {
	tests := []struct {
		name  string
		field string
		json  string
	}{
		{name: "timeout", field: "timeout", json: `{"limits": {"timeout": "soon"}}`},
		{name: "retry_base", field: "retry_base", json: `{"limits": {"retry_base": "eventually"}}`},
		{name: "retry_max", field: "retry_max", json: `{"limits": {"retry_max": "later"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFile(writeConfigFile(t, tt.json))
			if err == nil {
				t.Fatalf("load invalid %s duration error = nil, want validation error", tt.field)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("error = %v, want field %q", err, tt.field)
			}
		})
	}
}

func TestLoadFileRejectsNegativeNumericLimits(t *testing.T) {
	tests := []struct {
		field string
		json  string
	}{
		{field: "max_results", json: `{"limits": {"max_results": -1}}`},
		{field: "max_bytes", json: `{"limits": {"max_bytes": -1}}`},
		{field: "retries", json: `{"limits": {"retries": -1}}`},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			_, err := LoadFile(writeConfigFile(t, tt.json))
			if err == nil {
				t.Fatalf("load negative %s error = nil, want validation error", tt.field)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("error = %v, want field %q", err, tt.field)
			}
		})
	}
}

func TestLoadFileRejectsPresentEmptyBrowserBin(t *testing.T) {
	_, err := LoadFile(writeConfigFile(t, `{"browser": {"bin": ""}}`))
	if err == nil {
		t.Fatal("load empty browser.bin error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "browser.bin") {
		t.Fatalf("error = %v, want browser.bin context", err)
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
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}

	settings, err := LoadFile(writeConfigFile(t, `{"limits": {"max_results": 3}}`))
	if err != nil {
		t.Fatalf("load config file: %v", err)
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal loaded config: %v", err)
	}
	for name, value := range secrets {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("loaded config includes %s value %q in %s", name, value, encoded)
		}
	}
}

func assertUnsupportedConfigField(t *testing.T, err error, field string) {
	t.Helper()

	if err == nil {
		t.Fatalf("load config with unsupported field %q error = nil, want error", field)
	}
	if !strings.Contains(err.Error(), "unsupported config field") {
		t.Fatalf("error = %v, want unsupported config field context", err)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error = %v, want field %q", err, field)
	}
}

func assertErrorDoesNotContain(t *testing.T, err error, forbidden string) {
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
