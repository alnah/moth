package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/alnah/moth/internal/content"
)

func checkBrowser(
	ctx context.Context,
	options BrowserDoctorOptions,
	discoveryPlatform Platform,
	hintPlatform Platform,
	runner Runner,
) ToolStatus {
	resolved, err := ResolveBrowser(ctx, options, discoveryPlatform)
	if err != nil {
		return missingStatus(ToolChromium, installHints(ToolChromium, hintPlatform, nil))
	}

	status := okStatus(resolved)
	status.Version = detectVersion(ctx, runner, resolved, []string{"--version"})

	if options.DeepLaunch {
		status = deepLaunchBrowser(ctx, status, options.Launcher)
	}
	return status
}

// ResolveBrowser finds a Chromium-compatible browser executable without launching it.
func ResolveBrowser(ctx context.Context, options BrowserDoctorOptions, platform Platform) (ResolvedTool, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedTool{}, fmt.Errorf("resolve browser: %w", err)
	}
	platform = normalizeBrowserPlatform(platform)

	executableExists := browserExecutableExists(options)
	if options.ExplicitPath != "" {
		if executableExists(options.ExplicitPath) {
			return resolvedTool(ToolChromium, options.ExplicitPath, SourceExplicitPath), nil
		}
		return ResolvedTool{}, fmt.Errorf("resolve explicit browser path %q: %w", options.ExplicitPath, ErrToolMissing)
	}

	if envPath := os.Getenv("ROD_BROWSER_BIN"); envPath != "" {
		if executableExists(envPath) {
			return resolvedTool(ToolChromium, envPath, SourceEnvPath), nil
		}
		return ResolvedTool{}, fmt.Errorf("resolve browser from ROD_BROWSER_BIN=%q: %w", envPath, ErrToolMissing)
	}

	if options.SearchCommonInstallPaths || options.ExecutableExists != nil {
		for _, path := range commonBrowserInstallPaths(platform) {
			if err := ctx.Err(); err != nil {
				return ResolvedTool{}, fmt.Errorf("resolve browser: %w", err)
			}
			if executableExists(path) {
				return resolvedTool(ToolChromium, path, SourceCommonInstallPath), nil
			}
		}
	}

	for _, name := range browserExecutableNames() {
		resolved, err := Resolve(ctx, ResolveOptions{Name: ToolName(name)})
		if err == nil {
			resolved.Name = ToolChromium
			return resolved, nil
		}
	}

	if options.ManagedBrowserPath != "" {
		if executableExists(options.ManagedBrowserPath) {
			return resolvedTool(ToolChromium, options.ManagedBrowserPath, SourceRodManagedCache), nil
		}
		return ResolvedTool{}, fmt.Errorf("resolve Rod managed browser %q: %w", options.ManagedBrowserPath, ErrToolMissing)
	}

	return ResolvedTool{}, fmt.Errorf("resolve browser: %w", ErrToolMissing)
}

func normalizeBrowserPlatform(platform Platform) Platform {
	if platform.OS == "" {
		platform.OS = runtime.GOOS
	}
	return platform
}

func browserExecutableExists(options BrowserDoctorOptions) func(string) bool {
	if options.ExecutableExists != nil {
		return options.ExecutableExists
	}
	return isExistingExecutable
}

func browserExecutableNames() []string {
	return []string{"chromium", "chromium-browser", "google-chrome", "chrome", "msedge", "microsoft-edge"}
}

func commonBrowserInstallPaths(platform Platform) []string {
	switch platform.OS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		return windowsBrowserInstallPaths()
	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/microsoft-edge",
			"/snap/bin/chromium",
		}
	}
}

func windowsBrowserInstallPaths() []string {
	paths := make([]string, 0, 6)
	for _, root := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		paths = append(paths,
			filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(root, "Chromium", "Application", "chrome.exe"),
			filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
		)
	}
	return paths
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
