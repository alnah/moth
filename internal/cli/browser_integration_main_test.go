//go:build browser && !windows

package cli

import (
	"context"
	"os"
	"strings"
	"testing"
)

var browserIntegrationCommands = map[string]struct{}{
	"browser": {},
	"fetch":   {},
	"daemon":  {},
	"version": {},
}

// TestMain lets the test binary act as the CLI for integration subprocesses.
func TestMain(m *testing.M) {
	if browserIntegrationCommand(os.Args) {
		code := 0
		if err := Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
			code = 1
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func browserIntegrationCommand(args []string) bool {
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		return false
	}
	_, ok := browserIntegrationCommands[args[1]]
	return ok
}
