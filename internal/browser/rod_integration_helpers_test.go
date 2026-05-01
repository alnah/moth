//go:build browser

package browser

import (
	"errors"
	"os"
	"testing"
)

const browserRequiredEnv = "MOTH_BROWSER_REQUIRED"

func handleBrowserUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrBrowserMissing) {
		return
	}
	if os.Getenv(browserRequiredEnv) == "1" {
		t.Fatalf("browser unavailable while %s=1: %v", browserRequiredEnv, err)
	}
	t.Skipf("browser unavailable for browser-tag integration test: %v", err)
}
