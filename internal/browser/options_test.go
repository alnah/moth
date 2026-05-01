package browser

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOptionsConfigurePoolAndProfileDirectory(t *testing.T) {
	factory := func(context.Context) (Worker, error) { return &fakeWorker{}, nil }
	profileRoot := t.TempDir()

	pool := NewPool(1,
		WithWorkerFactory(factory),
		WithBrowserBin("/bin/browser"),
		WithNoSandbox(true),
		WithProfileRoot(profileRoot),
		WithProfileName(" work/profile:one "),
	)
	defer func() { _ = pool.Close() }()

	if pool.config.workerFactory == nil {
		t.Fatal("worker factory = nil, want configured factory")
	}
	if pool.config.browserBin != "/bin/browser" {
		t.Fatalf("browser bin = %q, want /bin/browser", pool.config.browserBin)
	}
	if !pool.config.noSandbox {
		t.Fatal("noSandbox = false, want true")
	}

	wantDir := filepath.Join(profileRoot, "profiles", "work_profile_one")
	if got := pool.config.userDataDir(); got != wantDir {
		t.Fatalf("user data dir = %q, want %q", got, wantDir)
	}
}

func TestSafeProfileNameUsesDefaultForBlankName(t *testing.T) {
	if got := safeProfileName(" \t\n "); got != "default" {
		t.Fatalf("safe profile name = %q, want default", got)
	}
}

func TestResolvedBrowserBinPrefersExplicitPath(t *testing.T) {
	t.Setenv("ROD_BROWSER_BIN", "/env/browser")

	if got := resolvedBrowserBin("/explicit/browser"); got != "/explicit/browser" {
		t.Fatalf("resolved browser bin = %q, want explicit path", got)
	}
	if got := resolvedBrowserBin(""); got != "/env/browser" {
		t.Fatalf("resolved browser bin from env = %q, want env path", got)
	}
}

func TestValidateBrowserBinRejectsMissingPathAndDirectory(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-browser")
	if err := validateBrowserBin(missingPath); err == nil {
		t.Fatal("validate missing browser error = nil, want error")
	}

	if err := validateBrowserBin(t.TempDir()); err == nil {
		t.Fatal("validate browser directory error = nil, want error")
	}
}

func TestValidateBrowserBinAcceptsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "browser")
	if err := os.WriteFile(path, []byte("browser"), 0o600); err != nil {
		t.Fatalf("write browser fixture: %v", err)
	}

	if err := validateBrowserBin(path); err != nil {
		t.Fatalf("validate browser file error = %v, want nil", err)
	}
}

func TestHeaderPairsAreStableAndSorted(t *testing.T) {
	got := headerPairs(map[string]string{"X-Z": "last", "A": "first"})
	want := []string{"A", "first", "X-Z", "last"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("header pairs = %#v, want %#v", got, want)
	}
}

func TestScreenshotReturnsWriteError(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}

	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{})))
	defer func() { _ = pool.Close() }()

	err := pool.Screenshot(context.Background(), ScreenshotRequest{
		URL:  "https://example.test/chart",
		Path: filepath.Join(parentFile, "chart.png"),
	})
	if err == nil {
		t.Fatal("Screenshot() error = nil, want write path error")
	}
	if !strings.Contains(err.Error(), "create screenshot directory") {
		t.Fatalf("Screenshot() error = %v, want create directory context", err)
	}
}
