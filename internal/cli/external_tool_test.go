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

func executeDefaultCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(ctx, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func buildTestExecutable(t *testing.T, binaryPath string, source string) {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write test executable source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	//nolint:gosec // Test builds a controlled executable from test source.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build test executable: %v\n%s", err, output)
	}
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()

	path := dir
	if current := os.Getenv("PATH"); current != "" {
		path = fmt.Sprintf("%s%c%s", dir, os.PathListSeparator, current)
	}
	t.Setenv("PATH", path)
}

func executableName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}
