package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// daemonSignalContext cancels the daemon context on SIGINT or SIGTERM.
func daemonSignalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}
