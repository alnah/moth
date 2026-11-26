package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alnah/moth/internal/browser"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultHTTPIdleTimeout   = 60 * time.Second
	defaultDaemonIdleTimeout = 5 * time.Minute
	defaultReapInterval      = 30 * time.Second
	defaultReapTimeout       = 30 * time.Second
)

type daemonProcess struct {
	runtimeDir     string
	lock           *runtimeLock
	listener       net.Listener
	httpServer     *http.Server
	server         *Server
	serveErr       chan error
	cancelRequests context.CancelFunc
	cancelReaper   context.CancelFunc
}

// Run starts a daemon and blocks until its context or a shutdown request.
func Run(ctx context.Context, options RunOptions) (runErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	daemon, err := startDaemon(ctx, options)
	if err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, daemon.close())
	}()
	return daemon.wait(ctx)
}

func startDaemon(ctx context.Context, options RunOptions) (*daemonProcess, error) {
	runtimeDir := strings.TrimSpace(options.RuntimeDir)
	if runtimeDir == "" {
		runtimeDir = defaultRuntimeDir()
	}
	if err := prepareRuntimeDir(runtimeDir); err != nil {
		return nil, err
	}

	runtimeLock, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		return nil, err
	}

	listener, network, address, err := listen(ctx, runtimeDir)
	if err != nil {
		_ = runtimeLock.Close()
		return nil, err
	}

	daemon := &daemonProcess{
		runtimeDir: runtimeDir,
		lock:       runtimeLock,
		listener:   listener,
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = daemon.close()
		}
	}()

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	pool := options.Pool
	if pool == nil {
		pool = browser.NewPool(browser.ResolvePoolSize(0), browser.WithBrowserBin(options.BrowserBin))
	}
	server := NewServer(ServerOptions{
		Version:    options.Version,
		Token:      token,
		StartedAt:  startedAt,
		Lifecycle:  options.Lifecycle,
		Logger:     options.Logger,
		Pool:       pool,
		BrowserBin: options.BrowserBin,
	})
	daemon.server = server
	httpServer, cancelRequests := newDaemonHTTPServer(server.Handler())
	daemon.cancelRequests = cancelRequests

	runtime := Runtime{
		Version:   options.Version,
		PID:       os.Getpid(),
		Network:   network,
		Address:   address,
		Token:     token,
		StartedAt: startedAt,
	}
	if err := writeRuntime(runtimeDir, runtime); err != nil {
		return nil, err
	}

	daemon.httpServer = httpServer
	daemon.serveErr = make(chan error, 1)
	go func() {
		daemon.serveErr <- httpServer.Serve(listener)
	}()

	idleTimeout := options.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultDaemonIdleTimeout
	}
	reapInterval := options.ReapInterval
	if reapInterval <= 0 {
		reapInterval = defaultReapInterval
	}
	reaperCtx, cancelReaper := context.WithCancel(ctx)
	daemon.cancelReaper = cancelReaper
	go daemon.reapLoop(reaperCtx, reapInterval, idleTimeout)

	cleanup = false
	return daemon, nil
}

// newDaemonHTTPServer builds the daemon HTTP server and cancels active
// requests when shutdown starts.
func newDaemonHTTPServer(handler http.Handler) (*http.Server, context.CancelFunc) {
	requestCtx, cancelRequests := context.WithCancel(context.Background())
	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultHTTPIdleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return requestCtx
		},
	}
	httpServer.RegisterOnShutdown(cancelRequests)
	return httpServer, cancelRequests
}

func (daemon *daemonProcess) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
	case <-daemon.server.done():
	case err := <-daemon.serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve daemon: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultServerTimeout)
	defer cancel()
	if err := daemon.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown daemon: %w", err)
	}
	return nil
}

func (daemon *daemonProcess) close() error {
	if daemon.cancelReaper != nil {
		daemon.cancelReaper()
	}
	if daemon.cancelRequests != nil {
		daemon.cancelRequests()
	}
	var closeErr error
	if daemon.server != nil {
		closeErr = daemon.server.Close()
	}
	// Publish the stopped state only after browser resources are closed.
	removeRuntime(daemon.runtimeDir)
	daemon.closeResources()
	return closeErr
}

func (daemon *daemonProcess) reapLoop(ctx context.Context, interval time.Duration, idleTimeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapCtx, cancel := context.WithTimeout(ctx, defaultReapTimeout)
			report, err := daemon.server.Reap(reapCtx)
			cancel()
			if err != nil {
				daemon.server.logger.Warn("reap browser lifecycle", "error", err)
			}
			if report.ClosedPages != 0 || report.ClosedSessions != 0 || report.ClosedBrowsers != 0 {
				daemon.server.logger.Debug("reaped browser lifecycle",
					"pages", report.ClosedPages,
					"sessions", report.ClosedSessions,
					"browsers", report.ClosedBrowsers,
				)
			}
			if daemon.server.Idle(idleTimeout) {
				daemon.server.requestShutdown()
				return
			}
		}
	}
}

func (daemon *daemonProcess) closeResources() {
	if daemon.listener != nil {
		_ = daemon.listener.Close()
	}
	if daemon.lock != nil {
		_ = daemon.lock.Close()
	}
}

// DefaultRuntimeDir returns the per-user daemon runtime directory.
func DefaultRuntimeDir() string {
	return defaultRuntimeDir()
}

func defaultRuntimeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".moth", "run")
}
