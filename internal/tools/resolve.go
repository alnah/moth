package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ResolveOptions configures tool path lookup.
type ResolveOptions struct {
	Name         ToolName
	ExplicitPath string
	EnvVar       string
	ToolsDir     string
}

// Resolve finds a tool using explicit path, environment path, tools dir, then PATH.
func Resolve(ctx context.Context, options ResolveOptions) (ResolvedTool, error) {
	_ = ctx

	if options.ExplicitPath != "" {
		if isExistingExecutable(options.ExplicitPath) {
			return resolvedTool(options.Name, options.ExplicitPath, SourceExplicitPath), nil
		}
		return ResolvedTool{}, fmt.Errorf(
			"resolve explicit %s path %q: %w",
			options.Name,
			options.ExplicitPath,
			ErrToolMissing,
		)
	}

	if options.EnvVar != "" {
		if envPath := os.Getenv(options.EnvVar); envPath != "" {
			if isExistingExecutable(envPath) {
				return resolvedTool(options.Name, envPath, SourceEnvPath), nil
			}
			return ResolvedTool{}, fmt.Errorf("resolve %s from %s=%q: %w", options.Name, options.EnvVar, envPath, ErrToolMissing)
		}
	}

	if options.ToolsDir != "" {
		for _, fileName := range toolFileNames(options.Name) {
			toolPath := filepath.Join(options.ToolsDir, fileName)
			if isExistingExecutable(toolPath) {
				return resolvedTool(options.Name, toolPath, SourceToolsDir), nil
			}
		}
	}

	path, err := exec.LookPath(string(options.Name))
	if err == nil {
		return resolvedTool(options.Name, path, SourcePATH), nil
	}

	return ResolvedTool{}, fmt.Errorf("resolve %s: %w", options.Name, ErrToolMissing)
}

func resolvedTool(name ToolName, path string, source ToolSource) ResolvedTool {
	return ResolvedTool{Name: name, Path: path, Source: source}
}

func isExistingExecutable(path string) bool {
	info, err := os.Stat(path) //nolint:gosec // Path is the user-selected executable candidate being checked.
	return err == nil && !info.IsDir()
}

func toolFileNames(name ToolName) []string {
	base := string(name)
	return []string{base, base + ".exe"}
}
