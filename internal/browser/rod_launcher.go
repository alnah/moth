package browser

import (
	"context"
	"os"

	"github.com/go-rod/rod/lib/launcher"
)

// newLeaklessChromeLauncher builds a Chrome launcher tied to the daemon
// lifetime by the leakless guard.
func newLeaklessChromeLauncher(ctx context.Context, headless bool) *launcher.Launcher {
	return launcher.New().
		Context(ctx).
		Headless(headless).
		Leakless(true)
}

// applyChromeLauncherOptions applies binary and sandbox settings shared by
// pool and persistent launchers.
func applyChromeLauncherOptions(chromeLauncher *launcher.Launcher, browserBin string, noSandbox bool) {
	if browserBin != "" {
		chromeLauncher.Bin(browserBin)
	}
	if noSandbox || os.Getenv("ROD_NO_SANDBOX") == "1" {
		chromeLauncher.NoSandbox(true)
	}
}
