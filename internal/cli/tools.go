package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/tools"
)

type toolsFlags struct {
	ToolsDir string
}

func addToolsCommand(root *cobra.Command, rootOptions *rootFlags) {
	options := toolsFlags{}

	toolsCmd := &cobra.Command{
		Use:   "tools",
		Short: "Inspect and manage external tools",
	}

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report external tool status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return newInvalidArgumentsError(errors.New("tools doctor accepts no positional arguments"))
			}
			ctx := cmd.Context()
			if rootOptions.Limits.Timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, rootOptions.Limits.Timeout)
				defer cancel()
			}

			report, err := tools.Doctor(ctx, tools.DoctorOptions{
				ToolsDir:                   options.ToolsDir,
				RequiredTesseractLanguages: []string{"eng", "fra"},
			})
			if err != nil {
				return fmt.Errorf("run tools doctor: %w", err)
			}

			if err := writeJSON(cmd.OutOrStdout(), rootOptions.Output.Pretty, report); err != nil {
				return fmt.Errorf("write tools doctor JSON: %w", err)
			}
			return nil
		},
	}
	doctorCmd.Flags().StringVar(&options.ToolsDir, "tools-dir", "", "directory containing user-local tool binaries")

	toolsCmd.AddCommand(doctorCmd)
	root.AddCommand(toolsCmd)
}
