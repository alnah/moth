package tools_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

func TestDoctorReportsMissingBrowserWithoutLaunchingChrome(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", "")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
	})
	if err != nil {
		t.Fatalf("run doctor without browser: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Status != tools.StatusMissing {
		t.Fatalf("browser status = %q, want missing", status.Status)
	}
	if status.Path != "" {
		t.Fatalf("browser path = %q, want empty path for missing browser", status.Path)
	}
	if !hasWarning(status.Warnings, content.WarningToolMissing) {
		t.Fatalf("browser warnings = %#v, want tool_missing", status.Warnings)
	}
	assertHintsMentionAny(t, status.InstallHints, []string{
		"brew", "apt", "dnf", "winget", "scoop", "choco", "docker", "wsl", "nix",
	})
}

func TestDoctorDiscoversBrowserFromPATHWhenRodBrowserBinIsUnset(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	pathBrowserDir := filepath.Join(t.TempDir(), "path-browser")
	pathBrowserPath := fakeExecutablePath(t, pathBrowserDir, "chromium")

	t.Setenv("PATH", pathBrowserDir)
	t.Setenv("ROD_BROWSER_BIN", "")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
	})
	if err != nil {
		t.Fatalf("run doctor with PATH browser: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Status != tools.StatusOK {
		t.Fatalf("browser status = %q, want ok", status.Status)
	}
	if filepath.Clean(status.Path) != filepath.Clean(pathBrowserPath) {
		t.Fatalf("browser path = %q, want PATH browser %q", status.Path, pathBrowserPath)
	}
	if status.Source != tools.SourcePATH {
		t.Fatalf("browser source = %q, want PATH/system lookup source", status.Source)
	}
	if status.Version == "" {
		t.Fatal("browser version is empty, want detected version")
	}
}

func TestDoctorDiscoversRodManagedBrowserCache(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	managedBrowserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "rod-cache"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", "")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			ManagedBrowserPath: managedBrowserPath,
		},
	})
	if err != nil {
		t.Fatalf("run doctor with managed browser cache: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Status != tools.StatusOK {
		t.Fatalf("browser status = %q, want ok", status.Status)
	}
	if filepath.Clean(status.Path) != filepath.Clean(managedBrowserPath) {
		t.Fatalf("browser path = %q, want managed cache browser %q", status.Path, managedBrowserPath)
	}
	if status.Source != tools.SourceRodManagedCache {
		t.Fatalf("browser source = %q, want Rod managed cache source", status.Source)
	}
}

func TestDoctorDeepBrowserLaunchUsesFakeLauncher(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")
	launcher := &fakeBrowserLauncher{}

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			DeepLaunch: true,
			Launcher:   launcher,
		},
	})
	if err != nil {
		t.Fatalf("run deep browser doctor: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Status != tools.StatusOK {
		t.Fatalf("browser status = %q, want ok", status.Status)
	}
	if len(status.Warnings) != 0 {
		t.Fatalf("browser warnings = %#v, want none", status.Warnings)
	}
	if len(launcher.paths) != 1 || filepath.Clean(launcher.paths[0]) != filepath.Clean(browserPath) {
		t.Fatalf("launcher paths = %#v, want single deep launch for %q", launcher.paths, browserPath)
	}
}

func TestDoctorDeepBrowserLaunchReportsFailureWithoutRealChrome(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")
	launcher := &fakeBrowserLauncher{err: errors.New("headless launch failed")}

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			DeepLaunch: true,
			Launcher:   launcher,
		},
	})
	if err != nil {
		t.Fatalf("run deep browser doctor with launch failure: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Status != tools.StatusWarning {
		t.Fatalf("browser status = %q, want warning", status.Status)
	}
	if filepath.Clean(status.Path) != filepath.Clean(browserPath) {
		t.Fatalf("browser path = %q, want resolved browser path %q", status.Path, browserPath)
	}
	if !hasWarning(status.Warnings, content.Warning("browser_launch_failed")) {
		t.Fatalf("browser warnings = %#v, want browser_launch_failed", status.Warnings)
	}
}

func TestDoctorDeepBrowserLaunchReportsTimeout(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")
	launcher := &fakeBrowserLauncher{err: context.DeadlineExceeded}

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			DeepLaunch: true,
			Launcher:   launcher,
		},
	})
	if err != nil {
		t.Fatalf("run deep browser doctor with timeout: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Status != tools.StatusWarning {
		t.Fatalf("browser status = %q, want warning", status.Status)
	}
	if !hasWarning(status.Warnings, content.WarningTimeout) {
		t.Fatalf("browser warnings = %#v, want timeout", status.Warnings)
	}
}

type fakeBrowserLauncher struct {
	err   error
	paths []string
}

func (launcher *fakeBrowserLauncher) Launch(ctx context.Context, path string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	launcher.paths = append(launcher.paths, path)
	return launcher.err
}
