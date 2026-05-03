package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// Execute builds the default CLI, runs args, and returns the command error without exiting.
func Execute(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := NewRootCommand(Dependencies{})
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.Execute()
}

func renderResult(cmd *cobra.Command, output outputFlags, value any) error {
	if output.OutputPath == "" {
		if err := writeJSON(cmd.OutOrStdout(), output.Pretty, value); err != nil {
			return fmt.Errorf("write command JSON: %w", err)
		}
		return nil
	}

	var data bytes.Buffer
	if err := writeJSON(&data, output.Pretty, value); err != nil {
		return fmt.Errorf("encode output JSON: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output.OutputPath), 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(output.OutputPath, data.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write output JSON: %w", err)
	}
	return nil
}

func commandContext(cmd *cobra.Command, options *rootFlags) (context.Context, context.CancelFunc) {
	ctx := cmd.Context()
	if options.Runtime.Limits.Timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, options.Runtime.Limits.Timeout)
}

func changedMaxResults(cmd *cobra.Command, options *rootFlags) int {
	if persistentFlagChanged(cmd.Root(), "max-results") || options.AppliedConfig.MaxResults {
		return options.Runtime.Limits.MaxResults
	}
	return 0
}

func requestTimeout(cmd *cobra.Command, options *rootFlags) time.Duration {
	if persistentFlagChanged(cmd.Root(), "timeout") || options.AppliedConfig.Timeout {
		return options.Runtime.Limits.Timeout
	}
	return 0
}
