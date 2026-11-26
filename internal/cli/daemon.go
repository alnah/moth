package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/supervisor"
)

type daemonFlags struct {
	RuntimeDir string
}

var runDaemon = supervisor.Run

var daemonErrorMappings = []struct {
	err  error
	code string
}{
	{err: supervisor.ErrDaemonUnavailable, code: "daemon_unavailable"},
	{err: supervisor.ErrDaemonVersion, code: "daemon_version_mismatch"},
	{err: supervisor.ErrDaemonAuth, code: "daemon_unauthorized"},
	{err: supervisor.ErrDaemonProtocol, code: "daemon_protocol_error"},
	{err: supervisor.ErrDaemonMethod, code: "daemon_method_failed"},
	{err: supervisor.ErrDaemonAlreadyRunning, code: "daemon_already_running"},
}

func addDaemonCommand(root *cobra.Command, rootOptions *rootFlags) {
	flags := &daemonFlags{}
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the local daemon",
	}
	daemonCmd.PersistentFlags().StringVar(&flags.RuntimeDir, "runtime-dir", "", "daemon runtime directory")
	daemonCmd.AddCommand(
		newDaemonRunCommand(rootOptions, flags),
		newDaemonStatusCommand(rootOptions, flags),
		newDaemonStopCommand(rootOptions, flags),
	)
	root.AddCommand(daemonCmd)
}

func newDaemonRunCommand(rootOptions *rootFlags, daemonOptions *daemonFlags) *cobra.Command {
	var browserBin string
	cmd := &cobra.Command{
		Use:    "run",
		Short:  "Run the local daemon",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("daemon run accepts no positional arguments"))
			}
			ctx, stop := daemonSignalContext(cmd.Context())
			defer stop()
			return runDaemon(ctx, supervisor.RunOptions{
				RuntimeDir: daemonOptions.runtimeDir(),
				Version:    version,
				BrowserBin: daemonBrowserBin(browserBin, rootOptions),
				LogPath:    supervisor.DefaultLogPath(),
			})
		},
	}
	cmd.Flags().StringVar(&browserBin, "browser-bin", "", "browser binary path")
	markCommandWithoutDependencies(cmd)
	return cmd
}

func newDaemonStatusCommand(rootOptions *rootFlags, daemonOptions *daemonFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the local daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("daemon status accepts no positional arguments"))
			}

			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			status, health := inspectDaemon(ctx, daemonOptions.runtimeDir())
			return renderResult(cmd, rootOptions.Output, daemonStatusResult(status, health))
		},
	}
	markCommandWithoutDependencies(cmd)
	return cmd
}

func newDaemonStopCommand(rootOptions *rootFlags, daemonOptions *daemonFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the local daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("daemon stop accepts no positional arguments"))
			}

			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			client := daemonOptions.client()
			defer func() { _ = client.Close() }()

			if err := client.Shutdown(ctx); err != nil {
				return fmt.Errorf("stop daemon: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, daemonStatusResult("stopped", nil))
		},
	}
	markCommandWithoutDependencies(cmd)
	return cmd
}

func inspectDaemon(ctx context.Context, runtimeDir string) (string, *supervisor.Health) {
	_, err := supervisor.ReadRuntime(runtimeDir)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "stale", nil
	}

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    version,
	})
	defer func() { _ = client.Close() }()

	health, err := client.Health(ctx)
	if err != nil {
		return "stale", nil
	}
	return "running", &health
}

func (flags *daemonFlags) runtimeDir() string {
	if flags == nil {
		return supervisor.DefaultRuntimeDir()
	}
	if runtimeDir := strings.TrimSpace(flags.RuntimeDir); runtimeDir != "" {
		return runtimeDir
	}
	return supervisor.DefaultRuntimeDir()
}

func (flags *daemonFlags) client() *supervisor.Client {
	return supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: flags.runtimeDir(),
		Version:    version,
	})
}

func daemonErrorCode(err error) (string, bool) {
	for _, mapping := range daemonErrorMappings {
		if errors.Is(err, mapping.err) {
			return mapping.code, true
		}
	}
	return "", false
}

func daemonBrowserBin(configured string, rootOptions *rootFlags) string {
	if configured != "" {
		return configured
	}
	_, environmentSettings, _ := config.LoadFromEnv(nil)
	return resolvedBrowserBin(rootOptions.Runtime.BrowserBin, environmentSettings.RodBrowserBin)
}

func markCommandWithoutDependencies(cmd *cobra.Command) {
	cmd.Annotations = map[string]string{"moth.no_dependencies": "true"}
}
