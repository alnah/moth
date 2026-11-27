package cli

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

func TestDaemonCommandBuildsArgs(t *testing.T) {
	cmd, err := daemonCommand(context.Background(), supervisor.DaemonSpec{
		RuntimeDir: "/tmp/moth-run",
		BrowserBin: "/opt/chrome",
	})
	if err != nil {
		t.Fatalf("daemonCommand() error = %v", err)
	}

	wantArgs := []string{
		"daemon", "run",
		"--runtime-dir", "/tmp/moth-run",
		"--browser-bin", "/opt/chrome",
	}
	if len(cmd.Args) != len(wantArgs)+1 {
		t.Fatalf("args = %#v, want executable plus %#v", cmd.Args, wantArgs)
	}
	if !reflect.DeepEqual(cmd.Args[1:], wantArgs) {
		t.Fatalf("args = %#v, want %#v", cmd.Args[1:], wantArgs)
	}
}

func TestDaemonCommandRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := daemonCommand(ctx, supervisor.DaemonSpec{RuntimeDir: "/tmp/moth-run"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("daemonCommand(canceled) error = %v, want context.Canceled", err)
	}
}

func TestDaemonCommandDoesNotUseCommandContext(t *testing.T) {
	cmd, err := daemonCommand(context.Background(), supervisor.DaemonSpec{RuntimeDir: "/tmp/moth-run"})
	if err != nil {
		t.Fatalf("daemonCommand() error = %v", err)
	}
	if cmd.Cancel != nil {
		t.Fatal("daemon command has a Cancel function, want detached process not killed with CLI context")
	}
}

func TestDaemonEnvironmentFiltersSecrets(t *testing.T) {
	got := daemonEnvironment([]string{
		"PATH=/usr/bin",
		"HOME=/Users/alnah",
		"USERPROFILE=/Users/alnah",
		"SystemRoot=C:\\Windows",
		"ProgramFiles=C:\\Program Files",
		"DISPLAY=:0",
		"ROD_NO_SANDBOX=1",
		"BRAVE_API_KEY=secret",
		"OPENAI_API_KEY=secret",
		"X_BEARER_TOKEN=secret",
		"PODCAST_INDEX_API_SECRET=secret",
		"GITHUB_TOKEN=secret",
	})

	allowed := []string{
		"PATH=/usr/bin",
		"HOME=/Users/alnah",
		"USERPROFILE=/Users/alnah",
		"SystemRoot=C:\\Windows",
		"ProgramFiles=C:\\Program Files",
		"DISPLAY=:0",
		"ROD_NO_SANDBOX=1",
	}
	for _, value := range allowed {
		if !containsEnv(got, value) {
			t.Fatalf("environment = %#v, want %q", got, value)
		}
	}
	for _, value := range []string{
		"BRAVE_API_KEY=secret",
		"OPENAI_API_KEY=secret",
		"X_BEARER_TOKEN=secret",
		"PODCAST_INDEX_API_SECRET=secret",
		"GITHUB_TOKEN=secret",
	} {
		if containsEnv(got, value) {
			t.Fatalf("environment = %#v, must not contain %q", got, value)
		}
	}
}

func containsEnv(environment []string, want string) bool {
	return slices.Contains(environment, want)
}
