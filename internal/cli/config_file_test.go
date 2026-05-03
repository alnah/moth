package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/limits"
	"github.com/alnah/moth/internal/pdf2txt"
	"github.com/alnah/moth/internal/webfetch"
	"github.com/alnah/moth/internal/websearch"
)

func TestConfigMaxResultsAppliesToSearchCommands(t *testing.T) {
	configPath := writeCLIConfigFile(t, `{"limits": {"max_results": 13}}`)

	tests := []struct {
		name   string
		args   []string
		assert func(*testing.T, *commandHarness)
	}{
		{
			name: "search web",
			args: []string{"--config", configPath, "search", "web", "moth"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.webSearch.options.Count != 13 {
					t.Fatalf("web search count = %d, want config max_results", harness.webSearch.options.Count)
				}
			},
		},
		{
			name: "youtube search",
			args: []string{"--config", configPath, "youtube", "search", "moth"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.youtube.search.MaxResults != 13 {
					t.Fatalf("youtube max results = %d, want config max_results", harness.youtube.search.MaxResults)
				}
			},
		},
		{
			name: "podcast search",
			args: []string{"--config", configPath, "podcast", "search", "moth"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.podcast.search.MaxResults != 13 {
					t.Fatalf("podcast search max results = %d, want config max_results", harness.podcast.search.MaxResults)
				}
			},
		},
		{
			name: "podcast episodes",
			args: []string{"--config", configPath, "podcast", "episodes", "42"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.podcast.episodes.MaxResults != 13 {
					t.Fatalf("podcast episodes max results = %d, want config max_results", harness.podcast.episodes.MaxResults)
				}
			},
		},
		{
			name: "x search",
			args: []string{"--config", configPath, "x", "search", "moth"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.x.search.MaxResults != 13 {
					t.Fatalf("x search max results = %d, want config max_results", harness.x.search.MaxResults)
				}
			},
		},
		{
			name: "x user",
			args: []string{"--config", configPath, "x", "user", "2244994945"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.x.user.MaxResults != 13 {
					t.Fatalf("x user max results = %d, want config max_results", harness.x.user.MaxResults)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			stdout, stderr, err := harness.execute(tt.args...)
			if err != nil {
				t.Fatalf("execute command: %v\nstderr: %s", err, stderr)
			}
			assertContentPackJSON(t, stdout)
			tt.assert(t, harness)
		})
	}
}

func TestConfigMaxBytesAppliesToBoundedAcquisitionCommands(t *testing.T) {
	configPath := writeCLIConfigFile(t, `{"limits": {"max_bytes": 8192}}`)

	tests := []struct {
		name   string
		args   []string
		assert func(*testing.T, *commandHarness)
	}{
		{
			name: "fetch",
			args: []string{"--config", configPath, "fetch", "https://example.test"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := webfetch.Request{URL: "https://example.test", MaxBytes: 8192}
				if !reflect.DeepEqual(harness.webFetch.request, want) {
					t.Fatalf("fetch request = %#v, want %#v", harness.webFetch.request, want)
				}
			},
		},
		{
			name: "podcast audio",
			args: []string{"--config", configPath, "podcast", "audio", "https://feed.test/rss.xml", "episode-guid"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.podcastAudio.request.MaxBytes != 8192 {
					t.Fatalf("podcast audio max bytes = %d, want config max_bytes", harness.podcastAudio.request.MaxBytes)
				}
			},
		},
		{
			name: "pdf2txt",
			args: []string{"--config", configPath, "pdf2txt", "paper.pdf"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := pdf2txt.Options{MaxTextBytes: 8192}
				if !reflect.DeepEqual(harness.pdf.options, want) {
					t.Fatalf("pdf2txt options = %#v, want %#v", harness.pdf.options, want)
				}
			},
		},
		{
			name: "browser screenshot",
			args: []string{"--config", configPath, "browser", "screenshot", "https://example.test", "shot.png"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.ScreenshotRequest{URL: "https://example.test", Path: "shot.png", MaxBytes: 8192}
				if !reflect.DeepEqual(harness.browser.screenshot, want) {
					t.Fatalf("screenshot request = %#v, want %#v", harness.browser.screenshot, want)
				}
			},
		},
		{
			name: "browser pdf",
			args: []string{"--config", configPath, "browser", "pdf", "https://example.test", "page.pdf"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.PDFRequest{URL: "https://example.test", Path: "page.pdf", MaxBytes: 8192}
				if !reflect.DeepEqual(harness.browser.pdf, want) {
					t.Fatalf("browser PDF request = %#v, want %#v", harness.browser.pdf, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			stdout, stderr, err := harness.execute(tt.args...)
			if err != nil {
				t.Fatalf("execute command: %v\nstderr: %s", err, stderr)
			}
			assertJSONType(t, stdout, stdoutTypeForConfigMaxBytesTest(tt.name))
			tt.assert(t, harness)
		})
	}
}

func TestConfigTimeoutCreatesCommandContextDeadline(t *testing.T) {
	configPath := writeCLIConfigFile(t, `{"limits": {"timeout": "4s"}}`)
	harness := newCommandHarness()
	started := time.Now()

	stdout, stderr, err := harness.execute("--config", configPath, "search", "web", "moth")
	if err != nil {
		t.Fatalf("execute search command: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)
	assertContextDeadlineNear(t, harness.webSearch.hadDeadline, harness.webSearch.deadline, started, 4*time.Second)
}

func TestConfigFlagPrecedence(t *testing.T) {
	t.Run("max results flag overrides config", func(t *testing.T) {
		harness := newCommandHarness()
		configPath := writeCLIConfigFile(t, `{"limits": {"max_results": 13}}`)

		stdout, stderr, err := harness.execute("--config", configPath, "--max-results", "5", "search", "web", "moth")
		if err != nil {
			t.Fatalf("execute search command: %v\nstderr: %s", err, stderr)
		}
		assertContentPackJSON(t, stdout)
		if harness.webSearch.options.Count != 5 {
			t.Fatalf("web search count = %d, want flag max-results", harness.webSearch.options.Count)
		}
	})

	t.Run("max bytes flag overrides config", func(t *testing.T) {
		harness := newCommandHarness()
		configPath := writeCLIConfigFile(t, `{"limits": {"max_bytes": 8192}}`)

		stdout, stderr, err := harness.execute("--config", configPath, "--max-bytes", "1024", "fetch", "https://example.test")
		if err != nil {
			t.Fatalf("execute fetch command: %v\nstderr: %s", err, stderr)
		}
		assertContentPackJSON(t, stdout)
		if harness.webFetch.request.MaxBytes != 1024 {
			t.Fatalf("fetch max bytes = %d, want flag max-bytes", harness.webFetch.request.MaxBytes)
		}
	})

	t.Run("timeout flag overrides config", func(t *testing.T) {
		harness := newCommandHarness()
		configPath := writeCLIConfigFile(t, `{"limits": {"timeout": "20s"}}`)
		started := time.Now()

		stdout, stderr, err := harness.execute("--config", configPath, "--timeout", "1s", "search", "web", "moth")
		if err != nil {
			t.Fatalf("execute search command: %v\nstderr: %s", err, stderr)
		}
		assertContentPackJSON(t, stdout)
		assertContextDeadlineNear(t, harness.webSearch.hadDeadline, harness.webSearch.deadline, started, time.Second)
	})
}

func TestConfigRetryValuesFeedDefaultDependencyAssemblyWithoutFlags(t *testing.T) {
	configPath := writeCLIConfigFile(t, `{
		"limits": {
			"retries": 2,
			"retry_base": "250ms",
			"retry_max": "2s"
		}
	}`)

	got, factoryCalled := executeWithDefaultDependencyFactory(t,
		"--config", configPath,
		"tools", "doctor",
	)

	if !factoryCalled {
		t.Fatal("default dependency factory was not called")
	}
	if got.Limits.Retries != 2 {
		t.Fatalf("default dependency retries = %d, want config retries", got.Limits.Retries)
	}
	if got.Limits.RetryBase != 250*time.Millisecond {
		t.Fatalf("default dependency retry base = %v, want config retry_base", got.Limits.RetryBase)
	}
	if got.Limits.RetryMax != 2*time.Second {
		t.Fatalf("default dependency retry max = %v, want config retry_max", got.Limits.RetryMax)
	}
}

func TestConfigValuesFeedDefaultDependencyAssembly(t *testing.T) {
	configPath := writeCLIConfigFile(t, `{
		"browser": {"bin": "/opt/moth/chrome"},
		"limits": {
			"timeout": "9s",
			"max_results": 13,
			"max_bytes": 8192,
			"retries": 1,
			"retry_base": "250ms",
			"retry_max": "2s"
		}
	}`)

	got, factoryCalled := executeWithDefaultDependencyFactory(t,
		"--config", configPath,
		"--retries", "5",
		"--retry-base", "1s",
		"--retry-max", "4s",
		"tools", "doctor",
	)
	if !factoryCalled {
		t.Fatal("default dependency factory was not called")
	}
	if got.BrowserBin != "/opt/moth/chrome" {
		t.Fatalf("default dependency browser bin = %q, want config browser.bin", got.BrowserBin)
	}
	wantLimits := limits.Options{
		Timeout:    9 * time.Second,
		MaxResults: 13,
		MaxBytes:   8192,
		Retries:    5,
		RetryBase:  time.Second,
		RetryMax:   4 * time.Second,
	}
	if !reflect.DeepEqual(got.Limits, wantLimits) {
		t.Fatalf("default dependency limits = %#v, want %#v", got.Limits, wantLimits)
	}
}

func TestConfigLoadErrorsRenderStableJSON(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.json")
	invalidPath := writeCLIConfigFile(t, `{"limits": {"timeout": "soon"}}`)
	unknownPath := writeCLIConfigFile(t, `{"limits": {"count": 10}}`)
	secretPath := writeCLIConfigFile(t, `{"openai_api_key": "must-not-load"}`)
	malformedPath := writeCLIConfigFile(t, `{"limits":`)

	tests := []struct {
		name               string
		path               string
		wantMessageParts   []string
		forbidMessageParts []string
	}{
		{name: "missing", path: missingPath, wantMessageParts: []string{"load config", missingPath}},
		{name: "invalid duration", path: invalidPath, wantMessageParts: []string{"load config", "timeout"}},
		{
			name:             "unknown field",
			path:             unknownPath,
			wantMessageParts: []string{"load config", "unsupported config field", "count"},
		},
		{
			name:               "secret field",
			path:               secretPath,
			wantMessageParts:   []string{"load config", "unsupported config field", "openai_api_key"},
			forbidMessageParts: []string{"must-not-load"},
		},
		{name: "malformed JSON", path: malformedPath, wantMessageParts: []string{"load config"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			stdout, stderr, err := harness.execute("--config", tt.path, "search", "web", "moth")
			if err == nil {
				t.Fatal("execute command error = nil, want config load error")
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty stdout for config load error", stdout)
			}

			document := decodeSingleJSONErrorDocument(t, stderr)
			if document.Error.Code != "invalid_arguments" {
				t.Fatalf("error.code = %q, want invalid_arguments", document.Error.Code)
			}
			for _, part := range tt.wantMessageParts {
				if !strings.Contains(document.Error.Message, part) {
					t.Fatalf("error.message = %q, want it to contain %q", document.Error.Message, part)
				}
			}
			for _, part := range tt.forbidMessageParts {
				if strings.Contains(document.Error.Message, part) {
					t.Fatalf("error.message = %q, want it to omit %q", document.Error.Message, part)
				}
			}
			if document.Warnings == nil {
				t.Fatal("warnings = nil, want empty array")
			}
			if len(document.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want empty array", document.Warnings)
			}
		})
	}
}

func TestConfigDoesNotAffectCommandsWhenFlagIsAbsent(t *testing.T) {
	harness := newCommandHarness()

	stdout, stderr, err := harness.execute("search", "web", "moth")
	if err != nil {
		t.Fatalf("execute search command without config: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)
	if harness.webSearch.options != (websearch.Options{Query: "moth"}) {
		t.Fatalf("websearch options = %#v, want no implicit config/default max_results override", harness.webSearch.options)
	}
}

func stdoutTypeForConfigMaxBytesTest(name string) string {
	switch name {
	case "browser screenshot", "browser pdf", "fetch", "podcast audio", "pdf2txt":
		return "content_pack"
	default:
		return ""
	}
}

func assertContextDeadlineNear(
	t *testing.T,
	hadDeadline bool,
	deadline time.Time,
	started time.Time,
	want time.Duration,
) {
	t.Helper()

	if !hadDeadline {
		t.Fatal("context deadline missing")
	}
	got := deadline.Sub(started)
	if got < want-500*time.Millisecond || got > want+500*time.Millisecond {
		t.Fatalf("context deadline after %v, want near %v", got, want)
	}
}

func executeWithDefaultDependencyFactory(t *testing.T, args ...string) (defaultDependencyOptions, bool) {
	t.Helper()

	var got defaultDependencyOptions
	factoryCalled := false
	previousFactory := defaultDependencyFactory
	defaultDependencyFactory = func(options defaultDependencyOptions) defaultDependencySet {
		factoryCalled = true
		got = options
		return defaultDependencySet{Dependencies: defaultTestDependencies()}
	}
	t.Cleanup(func() { defaultDependencyFactory = previousFactory })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := NewRootCommand(Dependencies{})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute command with default dependency seam: %v\nstderr: %s", err, stderr.String())
	}
	return got, factoryCalled
}

func writeCLIConfigFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "moth.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write CLI config fixture: %v", err)
	}
	return path
}
