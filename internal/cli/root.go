package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/limits"
)

// ErrUnknownCommand reports a command name that is not registered.
var ErrUnknownCommand = errors.New("unknown command")

type rootFlags struct {
	Output     outputFlags
	Limits     limits.Options
	ConfigPath string
	Verbose    bool
}

type outputFlags struct {
	JSON       bool
	Pretty     bool
	OutputPath string
}

type errorDocument struct {
	Type     string            `json:"type"`
	Error    errorDocumentBody `json:"error"`
	Warnings []string          `json:"warnings"`
}

type errorDocumentBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewRootCommand builds the testable root CLI without exiting the process.
func NewRootCommand() *cobra.Command {
	options := newRootFlags()

	cmd := &cobra.Command{
		Use:           "moth",
		Short:         "Moth content discovery CLI",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeUnknownCommandError(cmd, options.Output, args[0])
			}
			if err := cmd.Help(); err != nil {
				return fmt.Errorf("show root help: %w", err)
			}
			return nil
		},
	}

	flags := cmd.PersistentFlags()
	flags.BoolVar(&options.Output.JSON, "json", false, "write structured JSON output")
	flags.BoolVar(&options.Output.Pretty, "pretty", false, "pretty-print JSON output")
	flags.DurationVar(&options.Limits.Timeout, "timeout", options.Limits.Timeout, "command timeout")
	flags.IntVar(&options.Limits.MaxResults, "max-results", options.Limits.MaxResults, "maximum result count")
	flags.Int64Var(&options.Limits.MaxBytes, "max-bytes", options.Limits.MaxBytes, "maximum downloaded bytes")
	flags.StringVar(&options.Output.OutputPath, "output", "", "output path")
	flags.StringVar(&options.ConfigPath, "config", "", "config path")
	flags.BoolVar(&options.Verbose, "verbose", false, "enable verbose logs")
	flags.IntVar(&options.Limits.Retries, "retries", options.Limits.Retries, "retry count")
	flags.DurationVar(&options.Limits.RetryBase, "retry-base", options.Limits.RetryBase, "base retry delay")
	flags.DurationVar(&options.Limits.RetryMax, "retry-max", options.Limits.RetryMax, "maximum retry delay")

	addToolsCommand(cmd, options)

	return cmd
}

func newRootFlags() *rootFlags {
	return &rootFlags{
		Limits: limits.DefaultOptions(),
	}
}

func writeUnknownCommandError(cmd *cobra.Command, output outputFlags, commandName string) error {
	message := fmt.Sprintf("unknown command: %s", commandName)
	if output.JSON {
		document := errorDocument{
			Type: "error",
			Error: errorDocumentBody{
				Code:    "unknown_command",
				Message: message,
			},
			Warnings: []string{},
		}
		if err := writeJSON(cmd.ErrOrStderr(), output.Pretty, document); err != nil {
			return fmt.Errorf("write unknown command error: %w", err)
		}
	} else {
		cmd.PrintErrln(message)
	}

	return fmt.Errorf("%w: %s", ErrUnknownCommand, commandName)
}

func writeJSON(writer io.Writer, pretty bool, value any) error {
	encoder := json.NewEncoder(writer)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
