package browser

import (
	"context"
	"testing"

	"github.com/go-rod/rod/lib/launcher/flags"
)

func TestNewRodPersistentChromeLauncherUsesLeakless(t *testing.T) {
	t.Setenv("ROD_NO_SANDBOX", "")
	dataDir := t.TempDir()

	chromeLauncher := newRodPersistentChromeLauncher(context.Background(), LaunchRequest{
		DataDir:   dataDir,
		Show:      true,
		NoSandbox: true,
	}, "/fake/chrome")

	if got := chromeLauncher.Get(flags.UserDataDir); got != dataDir {
		t.Fatalf("UserDataDir = %q, want %q", got, dataDir)
	}
	if chromeLauncher.Has(flags.KeepUserDataDir) {
		t.Fatal("KeepUserDataDir flag is set, want non-managed launcher without managed-only flag")
	}
	if !chromeLauncher.Has(flags.Leakless) {
		t.Fatal("Leakless flag is not set, want Chrome owned by the daemon guard")
	}
	if chromeLauncher.Has(flags.Headless) {
		t.Fatal("Headless flag is set, want visible browser when Show is true")
	}
	if got := chromeLauncher.Get(flags.Bin); got != "/fake/chrome" {
		t.Fatalf("Bin = %q, want %q", got, "/fake/chrome")
	}
	if !chromeLauncher.Has(flags.NoSandbox) {
		t.Fatal("NoSandbox flag is not set")
	}
}
