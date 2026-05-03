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
	Output     outputFlags
	Limits     limits.Options
	ConfigPath string
	Config     configFlags
	Verbose    bool
}

type configFlags struct {
	BrowserBin string
	Timeout    bool
	MaxResults bool
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
		Limits: limits.DefaultOptions(),
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

	settings, err := config.LoadFile(options.ConfigPath)
	if err != nil {
		return newConfigLoadError(options.ConfigPath, err)
	}
	applyConfigSettings(cmd.Root(), options, settings)
	return nil
}

func applyConfigSettings(root *cobra.Command, options *rootFlags, settings config.FileSettings) {
	if settings.Presence.BrowserBin {
		options.Config.BrowserBin = settings.Browser.Bin
	}
	if settings.Presence.Limits.Timeout && !persistentFlagChanged(root, "timeout") {
		options.Limits.Timeout = settings.Limits.Timeout
		options.Config.Timeout = true
	}
	if settings.Presence.Limits.MaxResults && !persistentFlagChanged(root, "max-results") {
		options.Limits.MaxResults = settings.Limits.MaxResults
		options.Config.MaxResults = true
		if root.Annotations == nil {
			root.Annotations = map[string]string{}
		}
		root.Annotations["config.max_results"] = "true"
	}
	if settings.Presence.Limits.MaxBytes && !persistentFlagChanged(root, "max-bytes") {
		options.Limits.MaxBytes = settings.Limits.MaxBytes
	}
	if settings.Presence.Limits.Retries && !persistentFlagChanged(root, "retries") {
		options.Limits.Retries = settings.Limits.Retries
	}
	if settings.Presence.Limits.RetryBase && !persistentFlagChanged(root, "retry-base") {
		options.Limits.RetryBase = settings.Limits.RetryBase
	}
	if settings.Presence.Limits.RetryMax && !persistentFlagChanged(root, "retry-max") {
		options.Limits.RetryMax = settings.Limits.RetryMax
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
