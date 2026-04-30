package tools_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/tools"
)

func buildFakeToolProgram(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.go")
	binaryPath := filepath.Join(dir, executableFileName("fake-tool"))

	const source = `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		return
	}

	switch args[0] {
	case "echo-args":
		fmt.Println(strings.Join(args[1:], "\n"))
	case "fail":
		fmt.Fprintln(os.Stderr, "fake tool failed")
		os.Exit(7)
	case "write-output":
		if len(args) > 1 {
			fmt.Fprint(os.Stdout, args[1])
		}
		if len(args) > 2 {
			fmt.Fprint(os.Stderr, args[2])
		}
	case "wait-for-cancel":
		if len(args) > 1 {
			_ = os.WriteFile(args[1], []byte("ready\n"), 0o600)
		}
		time.Sleep(time.Hour)
	}
}
`

	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write fake process source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//nolint:gosec // Test builds a controlled fake process fixture.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake process program: %v\n%s", err, output)
	}

	return binaryPath
}

func fakeExecutablePath(t *testing.T, dir string, name string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // Test fixture directory must be searchable.
		t.Fatalf("create fake executable dir: %v", err)
	}

	path := filepath.Join(dir, executableFileName(name))
	contents := []byte("fake executable placeholder\n")
	//nolint:gosec // Test fixture must be executable for LookPath.
	if err := os.WriteFile(path, contents, 0o755); err != nil {
		t.Fatalf("write fake executable %s: %v", name, err)
	}

	return path
}

func executableFileName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}

	return name
}

func installFakeRequiredTools(t *testing.T, toolsDir string) {
	t.Helper()

	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		fakeExecutablePath(t, toolsDir, name)
	}
}

func isolatedPATH(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	return dir
}

type fakeDoctorRunner struct {
	tesseractLanguages []string
	commands           []tools.Command
}

func newFakeDoctorRunner(languages ...string) *fakeDoctorRunner {
	if len(languages) == 0 {
		languages = []string{"eng", "fra"}
	}

	return &fakeDoctorRunner{tesseractLanguages: languages}
}

func (runner *fakeDoctorRunner) Run(ctx context.Context, command tools.Command) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{ExitCode: -1}, err
	}

	runner.commands = append(runner.commands, command)
	if len(command.Args) > 0 && command.Args[0] == "--list-langs" {
		return tools.Result{Stdout: []byte(runner.tesseractLanguageOutput()), ExitCode: 0}, nil
	}

	return tools.Result{Stdout: []byte(fakeVersion(command.Path)), ExitCode: 0}, nil
}

func (runner *fakeDoctorRunner) tesseractLanguageOutput() string {
	return fmt.Sprintf(
		"List of available languages in fake tessdata (%d):\n%s\n",
		len(runner.tesseractLanguages),
		strings.Join(runner.tesseractLanguages, "\n"),
	)
}

func fakeVersion(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".exe")
	switch name {
	case "yt-dlp":
		return "2026.04.30\n"
	case "ffmpeg", "ffprobe":
		return name + " version 8.0\n"
	case "pdftotext":
		return "pdftotext version 25.11.0\n"
	case "ocrmypdf":
		return "ocrmypdf 16.99.0\n"
	case "tesseract":
		return "tesseract 5.5.1\n"
	case "chromium", "chrome", "google-chrome":
		return "Chromium 142.0.0 fake " + runtime.GOOS + "\n"
	default:
		return name + " version 1.0.0\n"
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for file %q", path)
}
