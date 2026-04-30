package tools_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	args := os.Args[1:]

	if len(args) > 0 {
		switch args[0] {
		case "echo-args":
			fmt.Println(strings.Join(args[1:], "\n"))
			return
		case "fail":
			fmt.Fprintln(os.Stderr, "fake tool failed")
			os.Exit(7)
		case "sleep":
			duration := 10 * time.Second
			if len(args) > 1 {
				seconds, err := strconv.Atoi(args[1])
				if err == nil {
					duration = time.Duration(seconds) * time.Second
				}
			}
			time.Sleep(duration)
			return
		case "--list-langs":
			langs := os.Getenv("MOTH_FAKE_TESSERACT_LANGS")
			if langs == "" {
				langs = "eng,fra"
			}
			fmt.Printf("List of available languages in fake tessdata (%d):\n", len(strings.Split(langs, ",")))
			for _, lang := range strings.Split(langs, ",") {
				fmt.Println(strings.TrimSpace(lang))
			}
			return
		case "--help", "-help", "-h", "-?":
			fmt.Printf("%s fake help\n", name)
			return
		case "--version", "-version", "-v":
			printVersion(name)
			return
		}
	}

	printVersion(name)
}

func printVersion(name string) {
	switch name {
	case "yt-dlp":
		fmt.Println("2026.04.30")
	case "ffmpeg", "ffprobe":
		fmt.Printf("%s version 8.0\n", name)
	case "pdftotext":
		fmt.Fprintln(os.Stderr, "pdftotext version 25.11.0")
	case "ocrmypdf":
		fmt.Println("ocrmypdf 16.99.0")
	case "tesseract":
		fmt.Println("tesseract 5.5.1")
	case "chromium", "chrome", "google-chrome":
		fmt.Printf("Chromium 142.0.0 fake %s\n", runtime.GOOS)
	default:
		fmt.Printf("%s version 1.0.0\n", name)
	}
}
`

	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write fake tool source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//nolint:gosec // Test builds a controlled fake tool fixture.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake tool program: %v\n%s", err, output)
	}

	return binaryPath
}

func installFakeTool(t *testing.T, programPath string, dir string, name string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // Test fixture directory must be searchable.
		t.Fatalf("create fake tool dir: %v", err)
	}

	contents, err := os.ReadFile(programPath) //nolint:gosec // Test reads the controlled fake tool fixture.
	if err != nil {
		t.Fatalf("read fake tool program: %v", err)
	}

	path := filepath.Join(dir, executableFileName(name))
	if err := os.WriteFile(path, contents, 0o755); err != nil { //nolint:gosec // Test fixture must be executable.
		t.Fatalf("write fake tool %s: %v", name, err)
	}

	return path
}

//nolint:unparam // Name documents the fake executable contract.
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

func installFakeRequiredTools(t *testing.T, programPath string, toolsDir string) {
	t.Helper()

	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "pdftotext", "ocrmypdf", "tesseract"} {
		installFakeTool(t, programPath, toolsDir, name)
	}
}

func isolatedPATH(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	return dir
}
