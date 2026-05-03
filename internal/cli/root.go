package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/limits"
)

// ErrUnknownCommand reports a command name that is not registered.
var ErrUnknownCommand = errors.New("unknown command")

type rootFlags struct {
	Output        outputFlags
	Runtime       config.RuntimeConfig
	ConfigPath    string
	AppliedConfig config.FieldSet
	Verbose       bool
}

type outputFlags struct {
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

type commandError struct {
	cause        error
	code         string
	message      string
	writeContext string
}

func (err *commandError) Error() string {
	return err.cause.Error()
}

func (err *commandError) Unwrap() error {
	return err.cause
}

type renderedCommandError struct {
	cause error
}

func (err renderedCommandError) Error() string {
	return err.cause.Error()
}

func (err renderedCommandError) Unwrap() error {
	return err.cause
}

// NewRootCommand builds the testable root CLI without exiting the process.
func NewRootCommand(deps Dependencies) *cobra.Command {
	options := newRootFlags()
	dependencyRuntime := newDefaultDependencyRuntime(options)

	cmd := &cobra.Command{
		Use:           "moth",
		Short:         "Moth content discovery CLI",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := loadConfigBeforeExecution(cmd, options); err != nil {
				return renderCommandError(cmd, options.Output, err)
			}
			dependencyRuntime.fill(&deps)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return newUnknownCommandError(args[0])
			}
			if err := cmd.Help(); err != nil {
				return fmt.Errorf("show root help: %w", err)
			}
			return nil
		},
	}

	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return renderCommandError(cmd, options.Output, newCommandError(
			"invalid_arguments",
			err.Error(),
			err,
			"write command parse error",
		))
	})

	flags := cmd.PersistentFlags()
	limitOptions := &options.Runtime.Limits
	flags.BoolVar(&options.Output.Pretty, "pretty", false, "pretty-print JSON output")
	flags.DurationVar(&limitOptions.Timeout, "timeout", limitOptions.Timeout, "command timeout")
	flags.IntVar(&limitOptions.MaxResults, "max-results", limitOptions.MaxResults, "maximum result count")
	flags.Int64Var(&limitOptions.MaxBytes, "max-bytes", limitOptions.MaxBytes, "maximum downloaded bytes")
	flags.StringVar(&options.Output.OutputPath, "output", "", "output path")
	flags.StringVar(&options.ConfigPath, "config", "", "config path")
	flags.BoolVar(&options.Verbose, "verbose", false, "enable verbose logs")
	flags.IntVar(&limitOptions.Retries, "retries", limitOptions.Retries, "retry count")
	flags.DurationVar(&limitOptions.RetryBase, "retry-base", limitOptions.RetryBase, "base retry delay")
	flags.DurationVar(&limitOptions.RetryMax, "retry-max", limitOptions.RetryMax, "maximum retry delay")

	addSearchCommand(cmd, options, &deps)
	addFetchCommand(cmd, options, &deps)
	addBrowserCommand(cmd, options, &deps)
	addYouTubeCommand(cmd, options, &deps)
	addPodcastCommand(cmd, options, &deps)
	addXCommand(cmd, options, &deps)
	addPDF2TextCommand(cmd, options, &deps)
	addTranscribeCommand(cmd, options, &deps)
	addToolsCommand(cmd, options, &deps)
	renderCommandErrors(cmd, &options.Output, dependencyRuntime.closeBrowserPool)

	return cmd
}

func newRootFlags() *rootFlags {
	return &rootFlags{
		Runtime: config.RuntimeConfig{Limits: limits.DefaultOptions()},
	}
}

func newUnknownCommandError(commandName string) error {
	return newCommandError(
		"unknown_command",
		fmt.Sprintf("unknown command: %s", commandName),
		fmt.Errorf("%w: %s", ErrUnknownCommand, commandName),
		"write unknown command error",
	)
}

func newInvalidArgumentsError(cause error) error {
	return newCommandError("invalid_arguments", cause.Error(), cause, "write command error")
}

func newConfigLoadError(path string, cause error) error {
	return newCommandError(
		"invalid_arguments",
		fmt.Sprintf("load config %q: %v", path, cause),
		cause,
		"write config error",
	)
}

func loadConfigBeforeExecution(cmd *cobra.Command, options *rootFlags) error {
	if options.ConfigPath == "" {
		return nil
	}

	fileConfig, err := config.LoadFile(options.ConfigPath)
	if err != nil {
		return newConfigLoadError(options.ConfigPath, err)
	}
	applyConfigSettings(cmd.Root(), options, fileConfig)
	return nil
}

func applyConfigSettings(root *cobra.Command, options *rootFlags, fileConfig config.FileConfig) {
	merged, applied := config.MergeFileConfig(options.Runtime, fileConfig, changedPersistentConfigFields(root))
	options.Runtime = merged
	options.AppliedConfig = applied
}

func changedPersistentConfigFields(root *cobra.Command) config.FieldSet {
	return config.FieldSet{
		Timeout:    persistentFlagChanged(root, "timeout"),
		MaxResults: persistentFlagChanged(root, "max-results"),
		MaxBytes:   persistentFlagChanged(root, "max-bytes"),
		Retries:    persistentFlagChanged(root, "retries"),
		RetryBase:  persistentFlagChanged(root, "retry-base"),
		RetryMax:   persistentFlagChanged(root, "retry-max"),
	}
}

func persistentFlagChanged(root *cobra.Command, name string) bool {
	flag := root.PersistentFlags().Lookup(name)
	return flag != nil && flag.Changed
}

func newCommandError(code string, message string, cause error, writeContext string) error {
	return &commandError{
		cause:        cause,
		code:         code,
		message:      message,
		writeContext: writeContext,
	}
}

func renderCommandErrors(command *cobra.Command, output *outputFlags, cleanup func() error) {
	if command.RunE != nil {
		run := command.RunE
		command.RunE = func(cmd *cobra.Command, args []string) error {
			runErr := run(cmd, args)
			cleanupErr := cleanup()
			if runErr != nil {
				return renderCommandError(cmd, *output, errors.Join(runErr, cleanupErr))
			}
			return nil
		}
	}

	for _, child := range command.Commands() {
		renderCommandErrors(child, output, cleanup)
	}
}

func renderCommandError(cmd *cobra.Command, output outputFlags, err error) error {
	var renderedErr renderedCommandError
	if errors.As(err, &renderedErr) {
		return err
	}

	code, message, writeContext := commandErrorFields(err)
	document := errorDocument{
		Type: "error",
		Error: errorDocumentBody{
			Code:    code,
			Message: message,
		},
		Warnings: []string{},
	}
	if writeErr := writeJSON(cmd.ErrOrStderr(), output.Pretty, document); writeErr != nil {
		return fmt.Errorf("%s: %w", writeContext, writeErr)
	}

	return renderedCommandError{cause: err}
}

func commandErrorFields(err error) (string, string, string) {
	var commandErr *commandError
	if errors.As(err, &commandErr) {
		return commandErr.code, commandErr.message, commandErr.writeContext
	}
	if errors.Is(err, browser.ErrBrowserStateUnavailable) {
		return "browser_state_unavailable", err.Error(), "write browser state error"
	}
	if errors.Is(err, browser.ErrBrowserStateCorrupt) {
		return "browser_state_corrupt", err.Error(), "write browser state error"
	}

	return "command_failed", err.Error(), "write command error"
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
