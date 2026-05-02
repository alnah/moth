package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

func TestDoctorReportsRequiredToolsWithPathsVersionsAndWarnings(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		fakeExecutablePath(t, toolsDir, name)
	}
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			DeepLaunch: false,
		},
	})
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}

	for _, name := range []tools.ToolName{
		tools.ToolYTDLP,
		tools.ToolFFmpeg,
		tools.ToolFFprobe,
		tools.ToolPDFToText,
		tools.ToolOCRMyPDF,
		tools.ToolTesseract,
		tools.ToolChromium,
	} {
		status := findToolStatus(t, report, name)
		if status.Status != tools.StatusOK {
			t.Fatalf("%s status = %q, want ok", name, status.Status)
		}
		if status.Path == "" {
			t.Fatalf("%s path is empty, want resolved executable path", name)
		}
		if status.Version == "" {
			t.Fatalf("%s version is empty, want detected version", name)
		}
		if len(status.Warnings) != 0 {
			t.Fatalf("%s warnings = %#v, want none", name, status.Warnings)
		}
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal doctor report: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode doctor report JSON: %v", err)
	}
	if document["type"] != "tool_doctor" {
		t.Fatalf("doctor report type = %v, want tool_doctor", document["type"])
	}
	if _, ok := document["tools"]; !ok {
		t.Fatalf("doctor report JSON missing tools: %s", encoded)
	}
	if _, ok := document["warnings"]; !ok {
		t.Fatalf("doctor report JSON missing warnings: %s", encoded)
	}
}

func TestDoctorProbesToolsThroughInjectedRunner(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		fakeExecutablePath(t, toolsDir, name)
	}
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")
	runner := newFakeDoctorRunner()

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     runner,
	})
	if err != nil {
		t.Fatalf("run doctor with injected runner: %v", err)
	}

	if status := findToolStatus(t, report, tools.ToolYTDLP); status.Version != "2026.04.30" {
		t.Fatalf("yt-dlp version = %q, want fake runner version", status.Version)
	}

	for _, probe := range []struct {
		tool tools.ToolName
		args []string
	}{
		{tool: tools.ToolYTDLP, args: []string{"--version"}},
		{tool: tools.ToolFFmpeg, args: []string{"--version"}},
		{tool: tools.ToolFFprobe, args: []string{"--version"}},
		{tool: tools.ToolPDFToText, args: []string{"-v"}},
		{tool: tools.ToolOCRMyPDF, args: []string{"--version"}},
		{tool: tools.ToolTesseract, args: []string{"--version"}},
		{tool: tools.ToolTesseract, args: []string{"--list-langs"}},
		{tool: tools.ToolChromium, args: []string{"--version"}},
	} {
		assertDoctorProbe(t, runner.commands, probe.tool, probe.args)
	}
}

func TestDoctorReportsMissingToolsWithoutInstalling(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "tesseract"} {
		fakeExecutablePath(t, toolsDir, name)
	}
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
	})
	if err != nil {
		t.Fatalf("run doctor with missing ocrmypdf: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolOCRMyPDF)
	if status.Status != tools.StatusMissing {
		t.Fatalf("ocrmypdf status = %q, want missing", status.Status)
	}
	if status.Path != "" {
		t.Fatalf("ocrmypdf path = %q, want empty path for missing tool", status.Path)
	}
	if !hasWarning(status.Warnings, content.WarningToolMissing) {
		t.Fatalf("ocrmypdf warnings = %#v, want tool_missing", status.Warnings)
	}
	if len(status.InstallHints) == 0 {
		t.Fatal("ocrmypdf install hints are empty, want manager-specific hint")
	}
	if status.Version != "" {
		t.Fatalf("ocrmypdf version = %q, want empty version for missing tool", status.Version)
	}
}

func TestDoctorReportsMissingTesseractLanguages(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		fakeExecutablePath(t, toolsDir, name)
	}
	browserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "browser"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner("eng"),
	})
	if err != nil {
		t.Fatalf("run doctor with missing tesseract language: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolTesseract)
	if status.Status != tools.StatusWarning {
		t.Fatalf("tesseract status = %q, want warning", status.Status)
	}
	if len(status.MissingLanguages) != 1 || status.MissingLanguages[0] != "fra" {
		t.Fatalf("missing languages = %#v, want [fra]", status.MissingLanguages)
	}
	if len(status.InstallHints) == 0 {
		t.Fatal("tesseract language install hints are empty, want manager-specific hint")
	}
}

func TestDoctorUsesRODBrowserBinBeforeOtherBrowserSources(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		fakeExecutablePath(t, toolsDir, name)
	}
	envBrowserPath := fakeExecutablePath(t, filepath.Join(t.TempDir(), "env-browser"), "chromium")
	pathBrowserDir := filepath.Join(t.TempDir(), "path-browser")
	fakeExecutablePath(t, pathBrowserDir, "chromium")

	t.Setenv("PATH", pathBrowserDir)
	t.Setenv("ROD_BROWSER_BIN", envBrowserPath)

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
		Runner:                     newFakeDoctorRunner(),
		Browser: tools.BrowserDoctorOptions{
			DeepLaunch: false,
		},
	})
	if err != nil {
		t.Fatalf("run browser doctor: %v", err)
	}

	status := findToolStatus(t, report, tools.ToolChromium)
	if filepath.Clean(status.Path) != filepath.Clean(envBrowserPath) {
		t.Fatalf("browser path = %q, want ROD_BROWSER_BIN path %q", status.Path, envBrowserPath)
	}
	if status.Source != tools.SourceEnvPath {
		t.Fatalf("browser source = %q, want env path", status.Source)
	}
}

func assertDoctorProbe(t *testing.T, commands []tools.Command, tool tools.ToolName, args []string) {
	t.Helper()

	for _, command := range commands {
		if command.Tool != tool || !slices.Equal(command.Args, args) {
			continue
		}
		if command.Path == "" {
			t.Fatalf("%s %v probe path is empty", tool, args)
		}
		if command.StdoutLimitBytes <= 0 || command.StderrLimitBytes <= 0 {
			t.Fatalf(
				"%s %v probe limits = stdout:%d stderr:%d, want positive limits",
				tool,
				args,
				command.StdoutLimitBytes,
				command.StderrLimitBytes,
			)
		}
		return
	}

	t.Fatalf("probe %s %v missing from injected runner commands: %#v", tool, args, commands)
}

func findToolStatus(t *testing.T, report tools.DoctorReport, name tools.ToolName) tools.ToolStatus {
	t.Helper()

	for _, status := range report.Tools {
		if status.Name == name {
			return status
		}
	}

	t.Fatalf("tool %s missing from doctor report: %#v", name, report.Tools)
	return tools.ToolStatus{}
}

func hasWarning(warnings []content.Warning, want content.Warning) bool {
	return slices.Contains(warnings, want)
}
