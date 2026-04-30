package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/limits"
)

// ErrUnknownCommand reports a command name that is not registered.
var ErrUnknownCommand = errors.New("unknown command")

type rootOptions struct {
	JSON       bool
	Pretty     bool
	Timeout    time.Duration
	MaxResults int
	MaxBytes   int64
	Output     string
	Config     string
	Verbose    bool
	Retries    int
	RetryBase  time.Duration
	RetryMax   time.Duration
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
	options := newRootOptions()

	cmd := &cobra.Command{
		Use:           "moth",
		Short:         "Moth content discovery CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeUnknownCommandError(cmd, options, args[0])
			}
			if err := cmd.Help(); err != nil {
				return fmt.Errorf("show root help: %w", err)
			}
			return nil
		},
	}

	flags := cmd.PersistentFlags()
	flags.BoolVar(&options.JSON, "json", false, "write structured JSON output")
	flags.BoolVar(&options.Pretty, "pretty", false, "pretty-print JSON output")
	flags.DurationVar(&options.Timeout, "timeout", options.Timeout, "command timeout")
	flags.IntVar(&options.MaxResults, "max-results", options.MaxResults, "maximum result count")
	flags.Int64Var(&options.MaxBytes, "max-bytes", options.MaxBytes, "maximum downloaded bytes")
	flags.StringVar(&options.Output, "output", "", "output path")
	flags.StringVar(&options.Config, "config", "", "config path")
	flags.BoolVar(&options.Verbose, "verbose", false, "enable verbose logs")
	flags.IntVar(&options.Retries, "retries", options.Retries, "retry count")
	flags.DurationVar(&options.RetryBase, "retry-base", options.RetryBase, "base retry delay")
	flags.DurationVar(&options.RetryMax, "retry-max", options.RetryMax, "maximum retry delay")

	return cmd
}

func newRootOptions() *rootOptions {
	defaults := limits.DefaultOptions()
	return &rootOptions{
		Timeout:    defaults.Timeout,
		MaxResults: defaults.MaxResults,
		MaxBytes:   defaults.MaxBytes,
		Retries:    defaults.Retries,
		RetryBase:  defaults.RetryBase,
		RetryMax:   defaults.RetryMax,
	}
}

func writeUnknownCommandError(cmd *cobra.Command, options *rootOptions, commandName string) error {
	message := fmt.Sprintf("unknown command: %s", commandName)
	if options.JSON {
		document := errorDocument{
			Type: "error",
			Error: errorDocumentBody{
				Code:    "unknown_command",
				Message: message,
			},
			Warnings: []string{},
		}
		if err := writeJSON(cmd.ErrOrStderr(), options.Pretty, document); err != nil {
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
