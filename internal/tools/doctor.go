package tools

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/alnah/moth/internal/content"
)

const (
	typeToolDoctor              = "tool_doctor"
	defaultBrowserLaunchTimeout = 5 * time.Second
	toolProbeOutputLimitBytes   = 64 * 1024
	toolVersionLimitBytes       = 256
)

// BrowserLauncher verifies a resolved browser executable without exposing Rod internals.
type BrowserLauncher interface {
	Launch(ctx context.Context, path string) error
}

// BrowserDoctorOptions configures Chromium/browser checks.
type BrowserDoctorOptions struct {
	ExplicitPath             string
	DeepLaunch               bool
	Launcher                 BrowserLauncher
	ManagedBrowserPath       string
	ExecutableExists         func(string) bool
	SearchCommonInstallPaths bool
}

// DoctorOptions configures all external tool checks.
type DoctorOptions struct {
	ToolsDir                   string
	RequiredTesseractLanguages []string
	Browser                    BrowserDoctorOptions
	Platform                   Platform
	Runner                     Runner
}

// Doctor checks required external tools and returns a JSON-ready report.
func Doctor(ctx context.Context, options DoctorOptions) (DoctorReport, error) {
	browserDiscoveryPlatform := options.Platform
	platform := options.Platform
	if platform.OS == "" {
		platform.OS = runtime.GOOS
	}
	runner := options.Runner
	if runner == nil {
		runner = LocalRunner{}
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
		status := checkTool(ctx, spec, options.ToolsDir, platform, runner)
		if spec.name == ToolTesseract && status.Status != StatusMissing {
			status = checkTesseractLanguages(ctx, runner, status, options.RequiredTesseractLanguages, platform)
		}
		report.Tools = append(report.Tools, status)
	}

	browserStatus := checkBrowser(ctx, options.Browser, browserDiscoveryPlatform, platform, runner)
	report.Tools = append(report.Tools, browserStatus)

	return report, nil
}

type toolSpec struct {
	name        ToolName
	envVar      string
	versionArgs []string
}

func requiredToolSpecs() []toolSpec {
	return []toolSpec{
		{name: ToolYTDLP, envVar: "YT_DLP_PATH", versionArgs: []string{"--version"}},
		{name: ToolFFmpeg, envVar: "FFMPEG_PATH", versionArgs: []string{"--version"}},
		{name: ToolFFprobe, envVar: "FFPROBE_PATH", versionArgs: []string{"--version"}},
		{name: ToolPDFToText, envVar: "PDFTOTEXT_PATH", versionArgs: []string{"-v"}},
		{name: ToolOCRMyPDF, envVar: "OCRMYPDF_PATH", versionArgs: []string{"--version"}},
		{name: ToolTesseract, envVar: "TESSERACT_PATH", versionArgs: []string{"--version"}},
	}
}

func checkTool(ctx context.Context, spec toolSpec, toolsDir string, platform Platform, runner Runner) ToolStatus {
	resolved, err := Resolve(ctx, ResolveOptions{
		Name:     spec.name,
		EnvVar:   spec.envVar,
		ToolsDir: toolsDir,
	})
	if err != nil {
		return missingStatus(spec.name, installHints(spec.name, platform, nil))
	}

	status := okStatus(resolved)
	status.Version = detectVersion(ctx, runner, resolved, spec.versionArgs)
	return status
}

func detectVersion(ctx context.Context, runner Runner, resolved ResolvedTool, args []string) string {
	result, _ := runner.Run(ctx, probeCommand(resolved, args))
	combined := strings.TrimSpace(strings.Join([]string{string(result.Stdout), string(result.Stderr)}, "\n"))
	return sanitizedVersionLine(strings.Split(combined, "\n")[0])
}

func sanitizedVersionLine(version string) string {
	version = strings.TrimSpace(version)
	for _, entry := range os.Environ() {
		_, value, ok := strings.Cut(entry, "=")
		if !ok || len(value) < 8 {
			continue
		}
		version = strings.ReplaceAll(version, value, "[redacted]")
	}
	if len(version) > toolVersionLimitBytes {
		version = version[:toolVersionLimitBytes]
	}
	return strings.TrimSpace(version)
}

func checkTesseractLanguages(
	ctx context.Context,
	runner Runner,
	status ToolStatus,
	required []string,
	platform Platform,
) ToolStatus {
	resolved := ResolvedTool{Name: status.Name, Path: status.Path}
	result, _ := runner.Run(ctx, probeCommand(resolved, []string{"--list-langs"}))

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

func probeCommand(resolved ResolvedTool, args []string) Command {
	return Command{
		Tool:             resolved.Name,
		Path:             resolved.Path,
		Args:             args,
		StdoutLimitBytes: toolProbeOutputLimitBytes,
		StderrLimitBytes: toolProbeOutputLimitBytes,
	}
}

func parseTesseractLanguages(output string) map[string]struct{} {
	languages := make(map[string]struct{})
	for line := range strings.SplitSeq(output, "\n") {
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
