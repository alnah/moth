package supervisor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alnah/moth/internal/browser"
)

// Reap runs one persistent browser lifecycle pass.
func (server *Server) Reap(ctx context.Context) (browser.ReapReport, error) {
	server.browserMu.Lock()
	defer server.browserMu.Unlock()

	report := browser.ReapReport{}
	if server.closed.Load() {
		return report, nil
	}
	var errs []error
	now := server.clock()
	for _, key := range sortedBrowserServiceKeys(server.browsers) {
		service := server.browsers[key]
		value, err := service.Reap(ctx, now)
		report.ClosedPages += value.ClosedPages
		report.ClosedSessions += value.ClosedSessions
		report.ClosedBrowsers += value.ClosedBrowsers
		if err != nil {
			errs = append(errs, fmt.Errorf("reap browser %s/%s: %w", key.Local, key.Global, err))
		}
	}
	return report, errors.Join(errs...)
}

// Idle reports whether the daemon can stop after the given inactivity.
func (server *Server) Idle(timeout time.Duration) bool {
	if server.activeCalls.Load() != 0 {
		return false
	}
	server.activityMu.Lock()
	lastActivity := server.lastActivity
	server.activityMu.Unlock()
	if server.clock().Sub(lastActivity) < timeout {
		return false
	}
	return server.activeBrowsers() == 0
}

// Shutdown requests a graceful daemon stop.
func (server *Server) Shutdown(context.Context) error {
	server.requestShutdown()
	return nil
}

// Close releases browser resources owned by the server.
func (server *Server) Close() error {
	server.closeOnce.Do(func() {
		server.cancelCalls()
		server.closed.Store(true)
		server.browserMu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), defaultServerTimeout)
		defer cancel()
		var errs []error
		for _, key := range sortedBrowserServiceKeys(server.browsers) {
			if err := server.browsers[key].Shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("close browser %s/%s: %w", key.Local, key.Global, err))
			}
		}
		clear(server.browsers)
		server.browserMu.Unlock()
		if server.pool != nil {
			errs = append(errs, server.pool.Close())
		}
		server.closeErr = errors.Join(errs...)
	})
	return server.closeErr
}

func (server *Server) done() <-chan struct{} {
	return server.shutdownCh
}

func (server *Server) requestShutdown() {
	server.shutdownOnce.Do(func() {
		close(server.shutdownCh)
	})
}

func (server *Server) callContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(server.callCtx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (server *Server) beginCall() {
	server.activeCalls.Add(1)
	server.activityMu.Lock()
	server.lastActivity = server.clock()
	server.activityMu.Unlock()
}

func (server *Server) endCall() {
	server.activeCalls.Add(-1)
}

func (server *Server) activeBrowsers() int {
	server.browserMu.Lock()
	defer server.browserMu.Unlock()
	active := 0
	for _, service := range server.browsers {
		active += service.ActiveBrowsers()
	}
	return active
}
