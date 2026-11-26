package supervisor

import (
	"path/filepath"
	"testing"

	"github.com/alnah/moth/internal/browser"
)

func TestCapturedByteCount(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
	}{
		{name: "int64", value: int64(4), want: 4},
		{name: "int", value: 5, want: 5},
		{name: "bytes", value: []byte("abc"), want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := capturedByteCount(tt.value)
			if err != nil || got != tt.want {
				t.Fatalf("capturedByteCount() = %d, %v, want %d", got, err, tt.want)
			}
		})
	}
	if _, err := capturedByteCount("bad"); err == nil {
		t.Fatal("capturedByteCount(string) error = nil, want error")
	}
}

func TestBrowserHeadersDefaultsWorkingDirAndStateDirs(t *testing.T) {
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	client := &Client{}

	headers, err := client.browserHeaders(true)
	if err != nil {
		t.Fatalf("browserHeaders() error = %v", err)
	}
	if headers[headerCWD] != workingDir {
		t.Fatalf("CWD = %q, want %q", headers[headerCWD], workingDir)
	}
	wantLocal := filepath.Join(workingDir, browser.DefaultStateDirs().Local)
	if headers[headerLocalStateDir] != wantLocal {
		t.Fatalf("local state dir = %q, want %q", headers[headerLocalStateDir], wantLocal)
	}
	if headers[headerGlobalStateDir] == "" {
		t.Fatal("global state dir is empty")
	}
}

func TestAbsoluteBrowserPath(t *testing.T) {
	workingDir := t.TempDir()
	got, err := absoluteBrowserPath(workingDir, "relative")
	if err != nil || got != filepath.Join(workingDir, "relative") {
		t.Fatalf("absoluteBrowserPath() = %q, %v", got, err)
	}
	if _, err := absoluteBrowserPath(workingDir, "bad\x00"); err == nil {
		t.Fatal("absoluteBrowserPath(NUL) error = nil, want error")
	}
}

func TestNewClientDefaultsRuntimeDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	client := NewClient(ClientOptions{})
	if client.options.RuntimeDir != DefaultRuntimeDir() {
		t.Fatalf("RuntimeDir = %q, want %q", client.options.RuntimeDir, DefaultRuntimeDir())
	}
}

func TestResolveClientPath(t *testing.T) {
	cwd := t.TempDir()
	tests := []struct {
		name    string
		cwd     string
		path    string
		want    string
		wantErr bool
	}{
		{name: "relative", cwd: cwd, path: "file.bin", want: filepath.Join(cwd, "file.bin")},
		{name: "absolute", cwd: cwd, path: filepath.Join(cwd, "file.bin"), want: filepath.Join(cwd, "file.bin")},
		{name: "empty", cwd: cwd, wantErr: true},
		{name: "relative cwd", cwd: "relative", path: "file.bin", wantErr: true},
		{name: "nul path", cwd: cwd, path: "bad\x00", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveClientPath(tt.cwd, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveClientPath() = %q, want error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("resolveClientPath() = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestClientWorkingDirRejectsInvalidPath(t *testing.T) {
	client := &Client{options: ClientOptions{WorkingDir: "bad\x00"}}
	if _, err := client.browserHeaders(true); err == nil {
		t.Fatal("browserHeaders(invalid working dir) error = nil, want error")
	}
}
