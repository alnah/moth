package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	logFile        *LogFile
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

	logFile, logger, err := openDaemonLogger(options)
	if err != nil {
		_ = runtimeLock.Close()
		return nil, err
	}

	listener, network, address, err := listen(ctx, runtimeDir)
	if err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		_ = runtimeLock.Close()
		return nil, err
	}

	daemon := &daemonProcess{
		runtimeDir: runtimeDir,
		lock:       runtimeLock,
		listener:   listener,
		logFile:    logFile,
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
		Logger:     logger,
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
	if logger != nil {
		logger.Info("daemon started",
			"runtime_dir", runtimeDir,
			"network", network,
			"address", address,
		)
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
	reason := "shutdown"
	var serveErr error
	select {
	case <-ctx.Done():
		reason = "context"
	case <-daemon.server.done():
	case err := <-daemon.serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			reason = "serve_error"
			serveErr = fmt.Errorf("serve daemon: %w", err)
		}
	}
	daemon.server.logger.Info("daemon stopping", "reason", reason)
	if serveErr != nil {
		return serveErr
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
		daemon.server.logger.Info("daemon stopped")
	}
	// Publish the stopped state only after browser resources are closed.
	removeRuntime(daemon.runtimeDir)
	closeErr = errors.Join(closeErr, daemon.closeResources())
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
				daemon.server.logger.Info("reaped browser lifecycle",
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

func (daemon *daemonProcess) closeResources() error {
	var errs []error
	if daemon.listener != nil {
		if err := daemon.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	if daemon.lock != nil {
		if err := daemon.lock.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if daemon.logFile != nil {
		if err := daemon.logFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DefaultRuntimeDir returns the per-user daemon runtime directory.
func DefaultRuntimeDir() string {
	return defaultRuntimeDir()
}

func defaultRuntimeDir() string {
	return filepath.Join(defaultMothDir(), "run")
}

func openDaemonLogger(options RunOptions) (*LogFile, *slog.Logger, error) {
	if options.Logger != nil {
		return nil, options.Logger, nil
	}
	logPath := strings.TrimSpace(options.LogPath)
	if logPath == "" {
		return nil, nil, nil
	}
	logFile, err := OpenLogFile(logPath, options.LogMaxBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("open daemon log: %w", err)
	}
	logger := logFile.Logger().With(
		"pid", os.Getpid(),
		"version", options.Version,
	)
	return logFile, logger, nil
}
