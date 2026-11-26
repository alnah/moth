package cli

import (
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
			return runDaemon(cmd.Context(), supervisor.RunOptions{
				RuntimeDir: daemonRuntimeDir(daemonOptions),
				Version:    version,
				BrowserBin: daemonBrowserBin(browserBin, rootOptions),
			})
		},
	}
	cmd.Flags().StringVar(&browserBin, "browser-bin", "", "browser binary path")
	cmd.Annotations = map[string]string{"moth.no_dependencies": "true"}
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

			runtimeDir := daemonRuntimeDir(daemonOptions)
			_, readErr := supervisor.ReadRuntime(runtimeDir)
			if errors.Is(readErr, os.ErrNotExist) {
				return renderResult(cmd, rootOptions.Output, daemonStatusResult("missing", nil))
			}
			if readErr != nil {
				return renderResult(cmd, rootOptions.Output, daemonStatusResult("stale", nil))
			}

			ctx, cancel := commandContext(cmd, rootOptions)
			defer cancel()
			client := supervisor.NewClient(supervisor.ClientOptions{
				RuntimeDir: runtimeDir,
				Version:    version,
			})
			defer func() { _ = client.Close() }()

			health, err := client.Health(ctx)
			if err != nil {
				return renderResult(cmd, rootOptions.Output, daemonStatusResult("stale", nil))
			}
			return renderResult(cmd, rootOptions.Output, daemonStatusResult("running", &health))
		},
	}
	cmd.Annotations = map[string]string{"moth.no_dependencies": "true"}
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
			client := supervisor.NewClient(supervisor.ClientOptions{
				RuntimeDir: daemonRuntimeDir(daemonOptions),
				Version:    version,
			})
			defer func() { _ = client.Close() }()

			if err := client.Shutdown(ctx); err != nil {
				return fmt.Errorf("stop daemon: %w", err)
			}
			return renderResult(cmd, rootOptions.Output, daemonStatusResult("stopped", nil))
		},
	}
	cmd.Annotations = map[string]string{"moth.no_dependencies": "true"}
	return cmd
}

func daemonRuntimeDir(flags *daemonFlags) string {
	if flags != nil {
		if runtimeDir := strings.TrimSpace(flags.RuntimeDir); runtimeDir != "" {
			return runtimeDir
		}
	}
	return supervisor.DefaultRuntimeDir()
}

func daemonBrowserBin(configured string, rootOptions *rootFlags) string {
	if configured != "" {
		return configured
	}
	_, environmentSettings, _ := config.LoadFromEnv(nil)
	return resolvedBrowserBin(rootOptions.Runtime.BrowserBin, environmentSettings.RodBrowserBin)
}
