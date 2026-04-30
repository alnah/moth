package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Command describes one external process execution without shell interpolation.
type Command struct {
	Path string
	Args []string
}

// Result captures a completed external process.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Run executes a command with context cancellation and captured output.
func Run(ctx context.Context, command Command) (Result, error) {
	result := Result{ExitCode: -1}

	//nolint:gosec // External tools are explicit executable paths, never shell strings.
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Stdout = stdout.Bytes()
	result.Stderr = stderr.Bytes()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("run %s: %w", command.Path, ctxErr)
	}
	if err == nil {
		return result, nil
	}
	if isMissingExecutableError(err) {
		return result, fmt.Errorf("run %s: %w", command.Path, ErrToolMissing)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result, fmt.Errorf("run %s: %w", command.Path, ErrToolFailed)
	}

	return result, fmt.Errorf("run %s: %w", command.Path, ErrToolFailed)
}

func isMissingExecutableError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound)
}
