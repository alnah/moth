package tools_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alnah/moth/internal/tools"
)

func TestResolveUsesExplicitEnvToolsDirThenPATH(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	explicitPath := fakeExecutablePath(t, filepath.Join(workspace, "explicit"), "yt-dlp")
	envPath := fakeExecutablePath(t, filepath.Join(workspace, "env"), "yt-dlp")
	toolsDir := filepath.Join(workspace, "tools")
	toolsDirPath := fakeExecutablePath(t, toolsDir, "yt-dlp")
	pathDir := filepath.Join(workspace, "path")
	pathToolPath := fakeExecutablePath(t, pathDir, "yt-dlp")

	t.Setenv("YT_DLP_PATH", envPath)
	t.Setenv("PATH", pathDir)

	cases := []struct {
		name         string
		options      tools.ResolveOptions
		wantPath     string
		wantSource   tools.ToolSource
		clearEnvPath bool
	}{
		{
			name: "explicit path wins over every other source",
			options: tools.ResolveOptions{
				Name:         tools.ToolYTDLP,
				ExplicitPath: explicitPath,
				EnvVar:       "YT_DLP_PATH",
				ToolsDir:     toolsDir,
			},
			wantPath:   explicitPath,
			wantSource: tools.SourceExplicitPath,
		},
		{
			name: "environment path wins when explicit path is absent",
			options: tools.ResolveOptions{
				Name:     tools.ToolYTDLP,
				EnvVar:   "YT_DLP_PATH",
				ToolsDir: toolsDir,
			},
			wantPath:   envPath,
			wantSource: tools.SourceEnvPath,
		},
		{
			name: "tools dir wins over PATH when explicit and environment paths are absent",
			options: tools.ResolveOptions{
				Name:     tools.ToolYTDLP,
				EnvVar:   "YT_DLP_PATH",
				ToolsDir: toolsDir,
			},
			wantPath:     toolsDirPath,
			wantSource:   tools.SourceToolsDir,
			clearEnvPath: true,
		},
		{
			name: "PATH is used after explicit environment and tools dir sources are absent",
			options: tools.ResolveOptions{
				Name:   tools.ToolYTDLP,
				EnvVar: "YT_DLP_PATH",
			},
			wantPath:     pathToolPath,
			wantSource:   tools.SourcePATH,
			clearEnvPath: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.clearEnvPath {
				t.Setenv("YT_DLP_PATH", "")
			}

			resolved, err := tools.Resolve(ctx, tc.options)
			if err != nil {
				t.Fatalf("resolve tool: %v", err)
			}

			if resolved.Name != tools.ToolYTDLP {
				t.Fatalf("tool name = %q, want %q", resolved.Name, tools.ToolYTDLP)
			}
			if filepath.Clean(resolved.Path) != filepath.Clean(tc.wantPath) {
				t.Fatalf("resolved path = %q, want %q", resolved.Path, tc.wantPath)
			}
			if resolved.Source != tc.wantSource {
				t.Fatalf("resolved source = %q, want %q", resolved.Source, tc.wantSource)
			}
		})
	}
}

func TestResolveMissingToolReturnsSemanticError(t *testing.T) {
	t.Setenv("YT_DLP_PATH", "")
	t.Setenv("PATH", isolatedPATH(t))

	_, err := tools.Resolve(context.Background(), tools.ResolveOptions{
		Name:     tools.ToolYTDLP,
		EnvVar:   "YT_DLP_PATH",
		ToolsDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("resolve missing tool error = nil, want tool_missing error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("resolve missing tool error = %v, want ErrToolMissing", err)
	}
}

func TestResolveRejectsExistingNonExecutableFileOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executable checks do not use Unix mode bits")
	}

	path := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(path, []byte("not executable\n"), 0o600); err != nil {
		t.Fatalf("write non-executable tool candidate: %v", err)
	}

	_, err := tools.Resolve(context.Background(), tools.ResolveOptions{
		Name:         tools.ToolYTDLP,
		ExplicitPath: path,
	})
	if err == nil {
		t.Fatal("resolve non-executable tool error = nil, want tool_missing error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("resolve non-executable tool error = %v, want ErrToolMissing", err)
	}

	_, err = tools.Run(context.Background(), tools.Command{Path: path})
	if err == nil {
		t.Fatal("run non-executable tool error = nil, want tool_missing error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("run non-executable tool error = %v, want ErrToolMissing", err)
	}
}

func TestResolveReturnsContextErrorWhenContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tools.Resolve(ctx, tools.ResolveOptions{
		Name:     tools.ToolYTDLP,
		ToolsDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("resolve with canceled context error = nil, want context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve with canceled context error = %v, want context canceled", err)
	}
	if errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("resolve with canceled context error = %v, must not report tool_missing", err)
	}
}
