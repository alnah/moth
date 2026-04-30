package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/alnah/moth/internal/content"
)

func checkBrowser(ctx context.Context, options BrowserDoctorOptions, platform Platform) ToolStatus {
	resolved, err := resolveBrowser(ctx, options)
	if err != nil {
		return missingStatus(ToolChromium, installHints(ToolChromium, platform, nil))
	}

	status := okStatus(resolved)
	status.Version = detectVersion(ctx, resolved.Path, "--version")

	if options.DeepLaunch {
		status = deepLaunchBrowser(ctx, status, options.Launcher)
	}
	return status
}

func resolveBrowser(ctx context.Context, options BrowserDoctorOptions) (ResolvedTool, error) {
	if envPath := os.Getenv("ROD_BROWSER_BIN"); envPath != "" {
		if isExistingExecutable(envPath) {
			return resolvedTool(ToolChromium, envPath, SourceEnvPath), nil
		}
		return ResolvedTool{}, fmt.Errorf("resolve browser from ROD_BROWSER_BIN=%q: %w", envPath, ErrToolMissing)
	}

	for _, name := range browserExecutableNames() {
		resolved, err := Resolve(ctx, ResolveOptions{Name: ToolName(name)})
		if err == nil {
			resolved.Name = ToolChromium
			return resolved, nil
		}
	}

	if options.ManagedBrowserPath != "" {
		if isExistingExecutable(options.ManagedBrowserPath) {
			return resolvedTool(ToolChromium, options.ManagedBrowserPath, SourceRodManagedCache), nil
		}
		return ResolvedTool{}, fmt.Errorf("resolve Rod managed browser %q: %w", options.ManagedBrowserPath, ErrToolMissing)
	}

	return ResolvedTool{}, fmt.Errorf("resolve browser: %w", ErrToolMissing)
}

func browserExecutableNames() []string {
	return []string{"chromium", "chromium-browser", "google-chrome", "chrome", "msedge", "microsoft-edge"}
}

func deepLaunchBrowser(ctx context.Context, status ToolStatus, launcher BrowserLauncher) ToolStatus {
	if launcher == nil {
		return status
	}

	launchCtx, cancel := context.WithTimeout(ctx, defaultBrowserLaunchTimeout)
	defer cancel()

	err := launcher.Launch(launchCtx, status.Path)
	if err == nil {
		return status
	}

	status.Status = StatusWarning
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		status.Warnings = append(status.Warnings, content.WarningTimeout)
		return status
	}
	status.Warnings = append(status.Warnings, content.Warning("browser_launch_failed"))
	return status
}
