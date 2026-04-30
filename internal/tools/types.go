package tools

import "github.com/alnah/moth/internal/content"

// ToolName identifies an external executable managed by Moth.
type ToolName string

// External tool names checked by doctor.
const (
	ToolYTDLP     ToolName = "yt-dlp"
	ToolFFmpeg    ToolName = "ffmpeg"
	ToolFFprobe   ToolName = "ffprobe"
	ToolPDFToText ToolName = "pdftotext"
	ToolOCRMyPDF  ToolName = "ocrmypdf"
	ToolTesseract ToolName = "tesseract"
	ToolChromium  ToolName = "chromium"
)

// ToolSource describes where a tool path was found.
type ToolSource string

// Tool resolution sources, ordered by precedence where applicable.
const (
	SourceExplicitPath    ToolSource = "explicit_path"
	SourceEnvPath         ToolSource = "env_path"
	SourceToolsDir        ToolSource = "tools_dir"
	SourcePATH            ToolSource = "path"
	SourceRodManagedCache ToolSource = "rod_managed_cache"
)

// ToolState is the doctor status for one external dependency.
type ToolState string

// Doctor status values.
const (
	StatusOK      ToolState = "ok"
	StatusMissing ToolState = "missing"
	StatusWarning ToolState = "warning"
)

// Platform identifies the target operating system for install hints.
type Platform struct {
	OS string
}

// ResolvedTool is an executable path with its resolution source.
type ResolvedTool struct {
	Name   ToolName   `json:"tool"`
	Path   string     `json:"path"`
	Source ToolSource `json:"source"`
}

// ToolStatus is one JSON doctor status entry.
type ToolStatus struct {
	Name             ToolName          `json:"tool"`
	Status           ToolState         `json:"status"`
	Path             string            `json:"path"`
	Source           ToolSource        `json:"source"`
	Version          string            `json:"version"`
	Warnings         []content.Warning `json:"warnings"`
	InstallHints     []string          `json:"install_hints"`
	MissingLanguages []string          `json:"missing_languages"`
}

// DoctorReport is the stable JSON document emitted by tools doctor.
type DoctorReport struct {
	Type     string            `json:"type"`
	Tools    []ToolStatus      `json:"tools"`
	Warnings []content.Warning `json:"warnings"`
}
