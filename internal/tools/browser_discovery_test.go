package tools_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/tools"
)

func TestDoctorDiscoversMacOSCommonBrowserApplications(t *testing.T) {
	const (
		googleChromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		chromiumPath     = "/Applications/Chromium.app/Contents/MacOS/Chromium"
		edgePath         = "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
	)

	cases := []struct {
		name string
		path string
	}{
		{name: "Google Chrome app", path: googleChromePath},
		{name: "Chromium app", path: chromiumPath},
		{name: "Microsoft Edge app", path: edgePath},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toolsDir := t.TempDir()
			installFakeRequiredTools(t, toolsDir)

			t.Setenv("PATH", isolatedPATH(t))
			t.Setenv("ROD_BROWSER_BIN", "")

			report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
				ToolsDir:                   toolsDir,
				RequiredTesseractLanguages: []string{"eng", "fra"},
				Platform:                   tools.Platform{OS: "darwin"},
				Runner:                     newFakeDoctorRunner(),
				Browser: tools.BrowserDoctorOptions{
					ExecutableExists: fakeExecutableExists(tc.path),
				},
			})
			if err != nil {
				t.Fatalf("run doctor with macOS common browser app: %v", err)
			}

			status := findToolStatus(t, report, tools.ToolChromium)
			if status.Status != tools.StatusOK {
				t.Fatalf("browser status = %q, want ok", status.Status)
			}
			if status.Path != tc.path {
				t.Fatalf("browser path = %q, want common app path %q", status.Path, tc.path)
			}
			if status.Source != tools.SourceCommonInstallPath {
				t.Fatalf("browser source = %q, want common install path", status.Source)
			}
			if status.Version == "" {
				t.Fatal("browser version is empty, want detected version")
			}
		})
	}
}

func TestDoctorBrowserExplicitPathWinsOverRodBrowserBinAndCommonInstallPaths(t *testing.T) {
	const commonChromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	explicitPath := "/opt/moth/browser/chrome"
	rodBrowserPath := "/opt/rod/chromium"

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", rodBrowserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Platform:                   tools.Platform{OS: "darwin"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			ExplicitPath:     explicitPath,
			ExecutableExists: fakeExecutableExists(explicitPath, rodBrowserPath, commonChromePath),
		},
	})
	if err != nil {
		t.Fatalf("run doctor with explicit browser path: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Path != explicitPath {
		t.Fatalf("browser path = %q, want explicit path %q", status.Path, explicitPath)
	}
	if status.Source != tools.SourceExplicitPath {
		t.Fatalf("browser source = %q, want explicit path", status.Source)
	}
}

func TestDoctorBrowserCommonInstallPathWinsOverPATH(t *testing.T) {
	programFiles := t.TempDir()
	commonChromePath := filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe")

	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	pathBrowserDir := filepath.Join(t.TempDir(), "path-browser")
	pathBrowserPath := fakeExecutablePath(t, pathBrowserDir, "chromium")

	t.Setenv("PATH", pathBrowserDir)
	t.Setenv("ROD_BROWSER_BIN", "")
	t.Setenv("PROGRAMFILES", programFiles)
	t.Setenv("PROGRAMFILES(X86)", "")
	t.Setenv("LOCALAPPDATA", "")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Platform:                   tools.Platform{OS: "windows"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			ExecutableExists: fakeExecutableExists(commonChromePath),
		},
	})
	if err != nil {
		t.Fatalf("run doctor with common browser path and PATH browser: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Path != commonChromePath {
		t.Fatalf(
			"browser path = %q, want common install path %q before PATH path %q",
			status.Path,
			commonChromePath,
			pathBrowserPath,
		)
	}
	if status.Source != tools.SourceCommonInstallPath {
		t.Fatalf("browser source = %q, want common install path", status.Source)
	}
}

func TestResolveBrowserDefaultsEmptyPlatformToCurrentOSForCommonInstallPaths(t *testing.T) {
	path := currentOSCommonBrowserPath(t)

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", "")

	resolved, err := tools.ResolveBrowser(context.Background(), tools.BrowserDoctorOptions{
		SearchCommonInstallPaths: true,
		ExecutableExists:         fakeExecutableExists(path),
	}, tools.Platform{})
	if err != nil {
		t.Fatalf("resolve browser with empty platform and %s common path: %v", runtime.GOOS, err)
	}

	if resolved.Path != path {
		t.Fatalf("browser path = %q, want current OS common path %q", resolved.Path, path)
	}
	if resolved.Source != tools.SourceCommonInstallPath {
		t.Fatalf("browser source = %q, want common install path", resolved.Source)
	}
}

func TestDoctorBrowserRodBrowserBinWinsOverCommonInstallPaths(t *testing.T) {
	const commonChromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	rodBrowserPath := "/opt/rod/chromium"

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", rodBrowserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Platform:                   tools.Platform{OS: "darwin"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			ExecutableExists: fakeExecutableExists(rodBrowserPath, commonChromePath),
		},
	})
	if err != nil {
		t.Fatalf("run doctor with ROD_BROWSER_BIN and common browser path: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if status.Path != rodBrowserPath {
		t.Fatalf("browser path = %q, want ROD_BROWSER_BIN path %q", status.Path, rodBrowserPath)
	}
	if status.Source != tools.SourceEnvPath {
		t.Fatalf("browser source = %q, want env path", status.Source)
	}
}

func TestDoctorBrowserVersionDoesNotLeakCredentialsOrUnboundedOutput(t *testing.T) {
	toolsDir := t.TempDir()
	installFakeRequiredTools(t, toolsDir)
	browserPath := fakeExecutablePath(t, t.TempDir(), "chromium")
	secret := "sk-test-secret-browser-token"

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)
	t.Setenv("OPENAI_API_KEY", secret)

	runner := &browserVersionRunner{
		version: "Google Chrome 142.0 token=" + secret + " " + strings.Repeat("x", 2048) + "\nignored second line\n",
	}
	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     runner,
	})
	if err != nil {
		t.Fatalf("run doctor with credential-looking browser version: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if strings.Contains(status.Version, secret) {
		t.Fatalf("browser version leaks credential %q in %q", secret, status.Version)
	}
	if len(status.Version) > 256 {
		t.Fatalf("browser version length = %d, want <= 256", len(status.Version))
	}
}

func currentOSCommonBrowserPath(t *testing.T) string {
	t.Helper()

	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "windows":
		programFiles := t.TempDir()
		t.Setenv("PROGRAMFILES", programFiles)
		t.Setenv("PROGRAMFILES(X86)", "")
		t.Setenv("LOCALAPPDATA", "")
		return filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe")
	default:
		return "/usr/bin/google-chrome"
	}
}

func fakeExecutableExists(paths ...string) func(string) bool {
	executables := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		executables[path] = struct{}{}
	}

	return func(path string) bool {
		_, ok := executables[path]
		return ok
	}
}

type browserVersionRunner struct {
	version string
}

func (runner *browserVersionRunner) Run(ctx context.Context, command tools.Command) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{ExitCode: -1}, err
	}
	if command.Tool == tools.ToolTesseract && len(command.Args) > 0 && command.Args[0] == "--list-langs" {
		return tools.Result{Stdout: []byte("eng\nfra\n"), ExitCode: 0}, nil
	}
	if command.Tool == tools.ToolChromium {
		return tools.Result{Stdout: []byte(runner.version), ExitCode: 0}, nil
	}

	return tools.Result{Stdout: []byte("tool version 1.0.0\n"), ExitCode: 0}, nil
}
