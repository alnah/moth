package ytdlp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alnah/moth/internal/content"
)

func parseMetadataItem(output []byte) (content.Item, error) {
	var metadata ytdlpMetadata
	if err := json.Unmarshal(output, &metadata); err != nil {
		return content.Item{}, fmt.Errorf("yt-dlp metadata decode: %w", err)
	}

	return mapMetadata(metadata), nil
}

func parseSubtitleFiles(output []byte, outputDir string) SubtitleFiles {
	return SubtitleFiles{Paths: outputPathLines(output, outputDir)}
}

func parseAudioFile(output []byte, outputDir string) (AudioFile, error) {
	paths := outputPathLines(output, outputDir)
	if len(paths) == 0 {
		return AudioFile{}, fmt.Errorf("yt-dlp audio: missing output path")
	}

	return AudioFile{Path: paths[len(paths)-1]}, nil
}

func isMissingSubtitleOutput(stderr []byte) bool {
	return strings.Contains(strings.ToLower(string(stderr)), "no subtitles")
}

func outputPathLines(output []byte, outputDir string) []string {
	paths := outputLines(output)
	cleanOutputDir := strings.TrimSpace(outputDir)
	controlledPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if cleanOutputDir == "" || isInsideOutputDir(path, cleanOutputDir) {
			controlledPaths = append(controlledPaths, path)
		}
	}

	return controlledPaths
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

func isInsideOutputDir(path string, outputDir string) bool {
	relativePath, err := filepath.Rel(filepath.Clean(outputDir), filepath.Clean(path))

	return err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}
