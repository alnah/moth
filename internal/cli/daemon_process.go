package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alnah/moth/internal/supervisor"
)

func startDaemonProcess(ctx context.Context, spec supervisor.DaemonSpec) error {
	command, err := daemonCommand(ctx, spec)
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start daemon process: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release daemon process: %w", err)
	}
	return nil
}

func daemonCommand(ctx context.Context, spec supervisor.DaemonSpec) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}

	args := []string{"daemon", "run", "--runtime-dir", spec.RuntimeDir}
	if spec.BrowserBin != "" {
		args = append(args, "--browser-bin", spec.BrowserBin)
	}

	//nolint:gosec,noctx // Executable is os.Executable and the daemon must outlive the CLI context.
	command := exec.Command(executable, args...)
	command.SysProcAttr = daemonSysProcAttr()
	command.Env = daemonEnvironment(os.Environ())
	return command, nil
}

var daemonEnvironmentAllowlist = map[string]struct{}{
	"PATH":            {},
	"HOME":            {},
	"USER":            {},
	"LOGNAME":         {},
	"SHELL":           {},
	"TMPDIR":          {},
	"TMP":             {},
	"TEMP":            {},
	"LANG":            {},
	"LC_ALL":          {},
	"LC_CTYPE":        {},
	"DISPLAY":         {},
	"WAYLAND_DISPLAY": {},
	"XDG_RUNTIME_DIR": {},
	"ROD_NO_SANDBOX":  {},
	"ROD_BROWSER_BIN": {},
}

func daemonEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := daemonEnvironmentAllowlist[name]; keep {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
