package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Runner executes one validated external command without shell interpolation.
type Runner interface {
	Run(ctx context.Context, command Command) (Result, error)
}

// LocalRunner runs external commands on the local machine.
type LocalRunner struct{}

// Command describes one external process execution without shell interpolation.
type Command struct {
	Tool             ToolName
	Path             string
	Args             []string
	StdoutLimitBytes int64
	StderrLimitBytes int64
}

// Result captures a completed external process.
type Result struct {
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	ExitCode        int
}

// Run executes a command with the default local runner.
func Run(ctx context.Context, command Command) (Result, error) {
	return LocalRunner{}.Run(ctx, command)
}

// Run executes a command with context cancellation and captured output.
func (LocalRunner) Run(ctx context.Context, command Command) (Result, error) {
	result := Result{ExitCode: -1}
	toolName := commandToolName(command)
	commandText := commandText(command)

	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("run %s %s: %w", toolName, commandText, err)
	}
	if err := validateExecutablePath(command.Path); err != nil {
		return result, fmt.Errorf("run %s %s: %w", toolName, commandText, errors.Join(ErrToolMissing, err))
	}

	//nolint:gosec // External tools are explicit executable paths, never shell strings.
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	stdout := newCapturedOutput(command.StdoutLimitBytes)
	stderr := newCapturedOutput(command.StderrLimitBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result.Stdout = stdout.Bytes()
	result.Stderr = stderr.Bytes()
	result.StdoutTruncated = stdout.Truncated()
	result.StderrTruncated = stderr.Truncated()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("run %s %s: %w", toolName, commandText, ctxErr)
	}
	if err == nil {
		return result, nil
	}
	if isMissingExecutableError(err) {
		return result, fmt.Errorf("run %s %s: %w", toolName, commandText, ErrToolMissing)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result, fmt.Errorf("run %s %s exited %d: %w", toolName, commandText, result.ExitCode, ErrToolFailed)
	}

	return result, fmt.Errorf("run %s %s: %w", toolName, commandText, errors.Join(ErrToolFailed, err))
}

func commandToolName(command Command) string {
	if command.Tool != "" {
		return string(command.Tool)
	}
	if command.Path != "" {
		return filepath.Base(command.Path)
	}
	return "external_tool"
}

func commandText(command Command) string {
	parts := make([]string, 0, 1+len(command.Args))
	parts = append(parts, strconv.Quote(command.Path))
	for _, arg := range command.Args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
}

func isMissingExecutableError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound)
}

type capturedOutput struct {
	buffer    bytes.Buffer
	limit     int64
	truncated bool
}

func newCapturedOutput(limit int64) *capturedOutput {
	return &capturedOutput{limit: limit}
}

func (output *capturedOutput) Write(p []byte) (int, error) {
	if output.limit <= 0 {
		if _, err := output.buffer.Write(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}

	remaining := output.limit - int64(output.buffer.Len())
	if remaining > 0 {
		writeLength := len(p)
		if int64(writeLength) > remaining {
			writeLength = int(remaining)
		}
		if _, err := output.buffer.Write(p[:writeLength]); err != nil {
			return 0, err
		}
	}
	if int64(len(p)) > remaining {
		output.truncated = true
	}

	return len(p), nil
}

func (output *capturedOutput) Bytes() []byte {
	return output.buffer.Bytes()
}

func (output *capturedOutput) Truncated() bool {
	return output.truncated
}
