package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

func TestDoctorReportsRequiredToolsWithPathsVersionsAndWarnings(t *testing.T) {
	programPath := buildFakeToolProgram(t)
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		installFakeTool(t, programPath, toolsDir, name)
	}
	browserPath := installFakeTool(t, programPath, filepath.Join(t.TempDir(), "browser"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)
	t.Setenv("MOTH_FAKE_TESSERACT_LANGS", "eng,fra")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
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

func TestDoctorReportsMissingToolsWithoutInstalling(t *testing.T) {
	programPath := buildFakeToolProgram(t)
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "tesseract"} {
		installFakeTool(t, programPath, toolsDir, name)
	}
	browserPath := installFakeTool(t, programPath, filepath.Join(t.TempDir(), "browser"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)
	t.Setenv("MOTH_FAKE_TESSERACT_LANGS", "eng,fra")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
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
	programPath := buildFakeToolProgram(t)
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		installFakeTool(t, programPath, toolsDir, name)
	}
	browserPath := installFakeTool(t, programPath, filepath.Join(t.TempDir(), "browser"), "chromium")

	t.Setenv("PATH", isolatedPATH(t))
	t.Setenv("ROD_BROWSER_BIN", browserPath)
	t.Setenv("MOTH_FAKE_TESSERACT_LANGS", "eng")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
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
	programPath := buildFakeToolProgram(t)
	toolsDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		installFakeTool(t, programPath, toolsDir, name)
	}
	envBrowserPath := installFakeTool(t, programPath, filepath.Join(t.TempDir(), "env-browser"), "chromium")
	pathBrowserDir := filepath.Join(t.TempDir(), "path-browser")
	installFakeTool(t, programPath, pathBrowserDir, "chromium")

	t.Setenv("PATH", pathBrowserDir)
	t.Setenv("ROD_BROWSER_BIN", envBrowserPath)
	t.Setenv("MOTH_FAKE_TESSERACT_LANGS", "eng,fra")

	report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
		ToolsDir:                   toolsDir,
		RequiredTesseractLanguages: []string{"eng", "fra"},
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
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}

	return false
}
