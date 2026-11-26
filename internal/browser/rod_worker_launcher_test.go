package browser

import (
	"context"
	"testing"

	"github.com/go-rod/rod/lib/launcher/flags"
)

func TestNewRodChromeLauncherUsesLeakless(t *testing.T) {
	t.Setenv("ROD_NO_SANDBOX", "")

	config := defaultPoolConfig()
	config.noSandbox = true
	chromeLauncher := newRodChromeLauncher(context.Background(), config, "/fake/chrome")

	if !chromeLauncher.Has(flags.Leakless) {
		t.Fatal("Leakless flag is not set, want Chrome owned by the daemon guard")
	}
	if !chromeLauncher.Has(flags.Headless) {
		t.Fatal("Headless flag is not set, want default headless worker")
	}
	if !chromeLauncher.Has(flags.NoSandbox) {
		t.Fatal("NoSandbox flag is not set, want configured no-sandbox")
	}
	if got := chromeLauncher.Get(flags.Bin); got != "/fake/chrome" {
		t.Fatalf("Bin = %q, want /fake/chrome", got)
	}
}
