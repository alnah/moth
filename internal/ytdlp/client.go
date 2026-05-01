// Package ytdlp wraps yt-dlp commands and maps metadata to content items.
package ytdlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

const defaultOutputLimitBytes = 10 << 20

// ErrSubtitlesMissing reports that yt-dlp found no subtitles for the request.
var ErrSubtitlesMissing = errors.New("subtitles_missing")

// Config contains yt-dlp dependencies.
type Config struct {
	ToolPath string
	Runner   tools.Runner
}

// MetadataRequest asks yt-dlp for video metadata only.
type MetadataRequest struct {
	URL string
}

// SubtitleRequest asks yt-dlp to write subtitle files.
type SubtitleRequest struct {
	URL              string
	OutputDir        string
	Languages        []string
	Format           string
	IncludeAutomatic bool
}

// AudioRequest asks yt-dlp to extract an audio file.
type AudioRequest struct {
	URL       string
	OutputDir string
	Format    string
	Section   TimeRange
}

// TimeRange selects one media section.
type TimeRange struct {
	Start time.Duration
	End   time.Duration
}

// SubtitleFiles lists subtitle paths printed by yt-dlp.
type SubtitleFiles struct {
	Paths []string
}

// AudioFile is the final audio file path printed by yt-dlp.
type AudioFile struct {
	Path string
}

// Client executes yt-dlp through the tools runner.
type Client struct {
	toolPath string
	runner   tools.Runner
}

// New creates a yt-dlp wrapper with defaults for unset dependencies.
func New(cfg Config) *Client {
	runner := cfg.Runner
	if runner == nil {
		runner = tools.LocalRunner{}
	}

	return &Client{
		toolPath: cfg.ToolPath,
		runner:   runner,
	}
}

// Metadata returns normalized video metadata from yt-dlp JSON output.
func (client *Client) Metadata(ctx context.Context, request MetadataRequest) (content.Item, error) {
	if err := validateRequestURL(request.URL); err != nil {
		return content.Item{}, err
	}
	if err := client.requireToolPath("metadata"); err != nil {
		return content.Item{}, err
	}

	result, err := client.run(ctx, []string{"-J", "--skip-download", request.URL})
	if err != nil {
		return content.Item{}, fmt.Errorf("yt-dlp metadata: %w", err)
	}

	var metadata ytdlpMetadata
	if err := json.Unmarshal(result.Stdout, &metadata); err != nil {
		return content.Item{}, fmt.Errorf("yt-dlp metadata decode: %w", err)
	}

	return mapMetadata(metadata), nil
}

// DownloadSubtitles writes subtitles and returns the paths printed by yt-dlp.
func (client *Client) DownloadSubtitles(ctx context.Context, request SubtitleRequest) (SubtitleFiles, error) {
	if err := validateRequestURL(request.URL); err != nil {
		return SubtitleFiles{}, err
	}
	if err := client.requireToolPath("subtitles"); err != nil {
		return SubtitleFiles{}, err
	}

	args := []string{"--skip-download", "--write-subs"}
	if request.IncludeAutomatic {
		args = append(args, "--write-auto-subs")
	}
	if len(request.Languages) > 0 {
		args = append(args, "--sub-langs", strings.Join(request.Languages, ","))
	}
	if request.Format != "" {
		args = append(args, "--sub-format", request.Format)
	}
	args = append(args,
		"--paths", request.OutputDir,
		"--output", "subtitle:%(id)s.%(language)s.%(ext)s",
		request.URL,
	)

	result, err := client.run(ctx, args)
	if err != nil {
		if isMissingSubtitleOutput(result.Stderr) {
			return SubtitleFiles{}, fmt.Errorf("yt-dlp subtitles: %w", ErrSubtitlesMissing)
		}
		return SubtitleFiles{}, fmt.Errorf("yt-dlp subtitles: %w", err)
	}

	return SubtitleFiles{Paths: outputLines(result.Stdout)}, nil
}

// ExtractAudio writes audio and returns the final file path printed by yt-dlp.
func (client *Client) ExtractAudio(ctx context.Context, request AudioRequest) (AudioFile, error) {
	if err := validateRequestURL(request.URL); err != nil {
		return AudioFile{}, err
	}
	if err := validateTimeRange(request.Section); err != nil {
		return AudioFile{}, err
	}
	if err := client.requireToolPath("audio"); err != nil {
		return AudioFile{}, err
	}

	args := []string{"--extract-audio"}
	if request.Format != "" {
		args = append(args, "--audio-format", request.Format)
	}
	if request.Section.Start > 0 || request.Section.End > 0 {
		args = append(args, "--download-sections", formatDownloadSection(request.Section))
	}
	args = append(args,
		"--paths", request.OutputDir,
		"--output", "%(id)s.%(ext)s",
		"--print", "after_move:filepath",
		request.URL,
	)

	result, err := client.run(ctx, args)
	if err != nil {
		return AudioFile{}, fmt.Errorf("yt-dlp audio: %w", err)
	}

	paths := outputLines(result.Stdout)
	if len(paths) == 0 {
		return AudioFile{}, fmt.Errorf("yt-dlp audio: missing output path")
	}

	return AudioFile{Path: paths[len(paths)-1]}, nil
}

func (client *Client) run(ctx context.Context, args []string) (tools.Result, error) {
	result, err := client.runner.Run(ctx, tools.Command{
		Tool:             tools.ToolYTDLP,
		Path:             client.toolPath,
		Args:             args,
		StdoutLimitBytes: defaultOutputLimitBytes,
		StderrLimitBytes: defaultOutputLimitBytes,
	})
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, tools.ErrToolFailed
	}

	return result, nil
}

func (client *Client) requireToolPath(operation string) error {
	if strings.TrimSpace(client.toolPath) == "" {
		return fmt.Errorf("yt-dlp %s: %w", operation, tools.ErrToolMissing)
	}

	return nil
}

func validateRequestURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("yt-dlp invalid url %q", rawURL)
	}

	return nil
}

func validateTimeRange(timeRange TimeRange) error {
	if timeRange.End > 0 && timeRange.Start > timeRange.End {
		return fmt.Errorf("yt-dlp duration range is invalid")
	}

	return nil
}

func isMissingSubtitleOutput(stderr []byte) bool {
	return strings.Contains(strings.ToLower(string(stderr)), "no subtitles")
}

func outputLines(output []byte) []string {
	lines := strings.Split(string(output), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		path := strings.TrimSpace(line)
		if path != "" {
			paths = append(paths, path)
		}
	}

	return paths
}

func formatDownloadSection(timeRange TimeRange) string {
	return "*" + formatSectionTime(timeRange.Start) + "-" + formatSectionTime(timeRange.End)
}

func formatSectionTime(duration time.Duration) string {
	totalSeconds := int(duration.Round(time.Second) / time.Second)
	hours := totalSeconds / 3600
	minutes := totalSeconds % 3600 / 60
	seconds := totalSeconds % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
