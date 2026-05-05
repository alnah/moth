package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDefaultDependenciesDiscoverYTDLPFromPATHForRuntimeCommands(t *testing.T) {
	toolsDir := t.TempDir()
	buildFakeYTDLPExecutable(t, filepath.Join(toolsDir, testExecutableName("yt-dlp")))
	prependTestPATH(t, toolsDir)

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
			stdout, stderr, err := executeDefaultRootCommand(t, tt.args...)
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

func executeDefaultRootCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(ctx, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func buildFakeYTDLPExecutable(t *testing.T, binaryPath string) {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), "main.go")
	const source = `package main

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
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write fake yt-dlp source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	//nolint:gosec // Test builds a controlled fake yt-dlp executable.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake yt-dlp: %v\n%s", err, output)
	}
}

func prependTestPATH(t *testing.T, dir string) {
	t.Helper()

	path := dir
	if current := os.Getenv("PATH"); current != "" {
		path = fmt.Sprintf("%s%c%s", dir, os.PathListSeparator, current)
	}
	t.Setenv("PATH", path)
}

func testExecutableName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}
