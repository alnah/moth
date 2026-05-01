// Package ytdlp wraps yt-dlp commands and maps metadata to content items.
package ytdlp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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

	result, err := client.run(ctx, metadataArgs(request))
	if err != nil {
		return content.Item{}, fmt.Errorf("yt-dlp metadata: %w", err)
	}

	return parseMetadataItem(result.Stdout)
}

// DownloadSubtitles writes subtitles and returns the paths printed by yt-dlp.
func (client *Client) DownloadSubtitles(ctx context.Context, request SubtitleRequest) (SubtitleFiles, error) {
	if err := validateRequestURL(request.URL); err != nil {
		return SubtitleFiles{}, err
	}
	if err := client.requireToolPath("subtitles"); err != nil {
		return SubtitleFiles{}, err
	}

	result, err := client.run(ctx, subtitleArgs(request))
	if err != nil {
		if isMissingSubtitleOutput(result.Stderr) {
			return SubtitleFiles{}, fmt.Errorf("yt-dlp subtitles: %w", ErrSubtitlesMissing)
		}
		return SubtitleFiles{}, fmt.Errorf("yt-dlp subtitles: %w", err)
	}

	return parseSubtitleFiles(result.Stdout, request.OutputDir), nil
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

	result, err := client.run(ctx, audioArgs(request))
	if err != nil {
		return AudioFile{}, fmt.Errorf("yt-dlp audio: %w", err)
	}

	return parseAudioFile(result.Stdout, request.OutputDir)
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
