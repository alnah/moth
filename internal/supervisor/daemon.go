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
)

type daemonProcess struct {
	runtimeDir string
	lock       *runtimeLock
	listener   net.Listener
	httpServer *http.Server
	server     *Server
	serveErr   chan error
}

// Run starts a daemon and blocks until its context or a shutdown request.
func Run(ctx context.Context, options RunOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	daemon, err := startDaemon(ctx, options)
	if err != nil {
		return err
	}
	defer daemon.close()
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
			daemon.closeResources()
		}
	}()

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	server := NewServer(ServerOptions{
		Version:   options.Version,
		Token:     token,
		StartedAt: startedAt,
	})
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

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
	daemon.server = server
	daemon.serveErr = make(chan error, 1)
	go func() {
		daemon.serveErr <- httpServer.Serve(listener)
	}()

	cleanup = false
	return daemon, nil
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

func (daemon *daemonProcess) close() {
	removeRuntime(daemon.runtimeDir)
	daemon.closeResources()
}

func (daemon *daemonProcess) closeResources() {
	if daemon.listener != nil {
		_ = daemon.listener.Close()
	}
	if daemon.lock != nil {
		_ = daemon.lock.Close()
	}
}

func defaultRuntimeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".moth", "run")
}
