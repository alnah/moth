package tools

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/alnah/moth/internal/content"
)

const (
	typeToolDoctor              = "tool_doctor"
	defaultBrowserLaunchTimeout = 5 * time.Second
)

// BrowserLauncher verifies a resolved browser executable without exposing Rod internals.
type BrowserLauncher interface {
	Launch(ctx context.Context, path string) error
}

// BrowserDoctorOptions configures Chromium/browser checks.
type BrowserDoctorOptions struct {
	DeepLaunch         bool
	Launcher           BrowserLauncher
	ManagedBrowserPath string
}

// DoctorOptions configures all external tool checks.
type DoctorOptions struct {
	ToolsDir                   string
	RequiredTesseractLanguages []string
	Browser                    BrowserDoctorOptions
	Platform                   Platform
}

// Doctor checks required external tools and returns a JSON-ready report.
func Doctor(ctx context.Context, options DoctorOptions) (DoctorReport, error) {
	platform := options.Platform
	if platform.OS == "" {
		platform.OS = runtime.GOOS
	}
	report := DoctorReport{
		Type:     typeToolDoctor,
		Tools:    make([]ToolStatus, 0, len(requiredToolSpecs())+1),
		Warnings: []content.Warning{},
	}
	for _, spec := range requiredToolSpecs() {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("run tools doctor: %w", err)
		}
		status := checkTool(ctx, spec, options.ToolsDir, platform)
		if spec.name == ToolTesseract && status.Status != StatusMissing {
			status = checkTesseractLanguages(ctx, status, options.RequiredTesseractLanguages, platform)
		}
		report.Tools = append(report.Tools, status)
	}

	browserStatus := checkBrowser(ctx, options.Browser, platform)
	report.Tools = append(report.Tools, browserStatus)

	return report, nil
}

type toolSpec struct {
	name       ToolName
	envVar     string
	versionArg string
}

func requiredToolSpecs() []toolSpec {
	return []toolSpec{
		{name: ToolYTDLP, envVar: "YT_DLP_PATH", versionArg: "--version"},
		{name: ToolFFmpeg, envVar: "FFMPEG_PATH", versionArg: "--version"},
		{name: ToolFFprobe, envVar: "FFPROBE_PATH", versionArg: "--version"},
		{name: ToolPDFToText, envVar: "PDFTOTEXT_PATH", versionArg: "--version"},
		{name: ToolOCRMyPDF, envVar: "OCRMYPDF_PATH", versionArg: "--version"},
		{name: ToolTesseract, envVar: "TESSERACT_PATH", versionArg: "--version"},
	}
}

func checkTool(ctx context.Context, spec toolSpec, toolsDir string, platform Platform) ToolStatus {
	resolved, err := Resolve(ctx, ResolveOptions{
		Name:     spec.name,
		EnvVar:   spec.envVar,
		ToolsDir: toolsDir,
	})
	if err != nil {
		return missingStatus(spec.name, installHints(spec.name, platform, nil))
	}

	status := okStatus(resolved)
	status.Version = detectVersion(ctx, resolved.Path, spec.versionArg)
	return status
}

func detectVersion(ctx context.Context, path string, versionArg string) string {
	result, _ := Run(ctx, Command{Path: path, Args: []string{versionArg}})
	combined := strings.TrimSpace(strings.Join([]string{string(result.Stdout), string(result.Stderr)}, "\n"))
	return strings.TrimSpace(strings.Split(combined, "\n")[0])
}

func checkTesseractLanguages(ctx context.Context, status ToolStatus, required []string, platform Platform) ToolStatus {
	result, _ := Run(ctx, Command{Path: status.Path, Args: []string{"--list-langs"}})

	available := parseTesseractLanguages(string(result.Stdout) + "\n" + string(result.Stderr))
	missing := missingLanguages(available, required)
	if len(missing) == 0 {
		return status
	}

	status.Status = StatusWarning
	status.Warnings = append(status.Warnings, content.Warning("tesseract_language_missing"))
	status.MissingLanguages = missing
	status.InstallHints = installHints(ToolTesseract, platform, missing)
	return status
}

func parseTesseractLanguages(output string) map[string]struct{} {
	languages := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "list of available languages") {
			continue
		}
		languages[line] = struct{}{}
	}
	return languages
}

func missingLanguages(available map[string]struct{}, required []string) []string {
	missing := make([]string, 0)
	for _, language := range required {
		language = strings.TrimSpace(language)
		if _, ok := available[language]; !ok {
			missing = append(missing, language)
		}
	}
	return missing
}

func okStatus(resolved ResolvedTool) ToolStatus {
	return ToolStatus{
		Name:             resolved.Name,
		Status:           StatusOK,
		Path:             resolved.Path,
		Source:           resolved.Source,
		Warnings:         []content.Warning{},
		InstallHints:     []string{},
		MissingLanguages: []string{},
	}
}

func missingStatus(name ToolName, hints []string) ToolStatus {
	return ToolStatus{
		Name:             name,
		Status:           StatusMissing,
		Warnings:         []content.Warning{content.WarningToolMissing},
		InstallHints:     hints,
		MissingLanguages: []string{},
	}
}
