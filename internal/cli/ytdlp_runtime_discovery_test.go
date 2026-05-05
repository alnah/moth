package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultDependenciesDiscoverYTDLPFromPATHForRuntimeCommands(t *testing.T) {
	toolsDir := t.TempDir()
	buildTestExecutable(t, filepath.Join(toolsDir, executableName("yt-dlp")), fakeYTDLPSource)
	prependPATH(t, toolsDir)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "subtitles",
			args: []string{
				"youtube", "subtitles", "https://youtu.be/video123",
				"--output-dir", filepath.Join(t.TempDir(), "subtitles"),
			},
		},
		{
			name: "audio",
			args: []string{
				"youtube", "audio", "https://youtu.be/video123",
				"--output-dir", filepath.Join(t.TempDir(), "audio"),
				"--format", "mp3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeDefaultCLI(t, tt.args...)
			if err != nil {
				t.Fatalf("execute %q with PATH yt-dlp: %v\nstderr: %s", strings.Join(tt.args, " "), err, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty stderr", stderr)
			}
			assertJSONType(t, stdout, "content_pack")
		})
	}
}

const fakeYTDLPSource = `package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	args := os.Args[1:]
	switch {
	case hasArg(args, "--write-subs"):
		outputDir := flagValue(args, "--paths")
		if outputDir == "" {
			outputDir = "."
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			panic(err)
		}
		path := filepath.Join(outputDir, "video123.en.vtt")
		if err := os.WriteFile(path, []byte("WEBVTT\n"), 0o600); err != nil {
			panic(err)
		}
		fmt.Println(path)
	case hasArg(args, "--extract-audio"):
		outputDir := flagValue(args, "--paths")
		if outputDir == "" {
			outputDir = "."
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			panic(err)
		}
		path := filepath.Join(outputDir, "video123.mp3")
		if err := os.WriteFile(path, []byte("fake mp3\n"), 0o600); err != nil {
			panic(err)
		}
		fmt.Println(path)
	default:
		fmt.Fprintln(os.Stderr, "unsupported fake yt-dlp invocation")
		os.Exit(64)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}
`
