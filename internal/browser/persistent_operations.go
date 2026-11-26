package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Start launches an owned browser unless current state is still running.
func (service *PersistentService) Start(ctx context.Context, request StartRequest) (BrowserStatus, error) {
	scope := service.writeScope(request.Scope)
	state, readErr := service.loadState(scope)
	switch {
	case readErr == nil:
		if _, connectErr := service.connectAndCache(ctx, scope, state.DebugURL); connectErr == nil {
			service.touchBrowser(scope, service.clock(), state.Owned)
			return state.status("running"), nil
		} else if ctxErr := ctx.Err(); ctxErr != nil {
			return BrowserStatus{}, ctxErr
		}
		service.invalidateConnection(scope)
		if state.Owned {
			if err := service.terminateOwned(ctx, state.ChromePID); err != nil {
				return BrowserStatus{}, err
			}
		}
	case !errors.Is(readErr, os.ErrNotExist) && !errors.Is(readErr, ErrBrowserStateCorrupt):
		return BrowserStatus{}, readErr
	default:
		service.invalidateConnection(scope)
	}

	stateDir := service.stateDir(scope)
	dataDir := filepath.Join(stateDir, persistentChromeDirName)
	result, err := service.launcher.LaunchBrowser(ctx, LaunchRequest{
		DataDir:    dataDir,
		Show:       request.Show,
		BrowserBin: service.browserBin,
		NoSandbox:  service.noSandbox,
	})
	if err != nil {
		return BrowserStatus{}, fmt.Errorf("launch persistent browser: %w", err)
	}
	state = persistentBrowserState{
		Scope:     scope,
		DebugURL:  result.DebugURL,
		ChromePID: result.ChromePID,
		Owned:     true,
		DataDir:   dataDir,
	}
	connection, connectErr := service.connectAndCache(ctx, scope, state.DebugURL)
	if saveErr := service.saveState(state); saveErr != nil {
		service.invalidateConnection(scope)
		service.deleteBrowser(scope)
		if connectErr == nil {
			closeErr := connection.Close(ctx)
			return BrowserStatus{}, errors.Join(saveErr, closeErr)
		}
		termErr := service.terminateOwned(ctx, state.ChromePID)
		return BrowserStatus{}, errors.Join(saveErr, termErr)
	}
	service.resetBrowser(scope, service.clock(), true)
	// A saved state is usable even when the eager cache fill failed.
	//nolint:nilerr // connection errors are retried by the next command.
	if connectErr != nil {
		return state.status("running"), nil
	}
	return state.status("running"), nil
}

// Stop clears state and closes only owned browsers.
func (service *PersistentService) Stop(ctx context.Context, request StopRequest) (BrowserStatus, error) {
	scope := service.readScope(request.Scope)
	state, err := service.loadState(scope)
	if errors.Is(err, os.ErrNotExist) {
		service.invalidateConnection(scope)
		service.deleteBrowser(scope)
		return BrowserStatus{Status: "stopped", Scope: scope}, nil
	}
	if err != nil {
		return BrowserStatus{}, err
	}

	connection, ok := service.cachedConnection(scope)
	if !ok || connection.DebugURL != state.DebugURL {
		if connected, connectErr := service.connectionFor(ctx, scope, state.DebugURL); connectErr == nil {
			connection = cachedBrowserConnection{DebugURL: state.DebugURL, Connection: connected}
			ok = true
		}
	}
	if state.Owned {
		switch {
		case ok:
			closeErr := connection.Connection.Close(ctx)
			if closeErr != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return BrowserStatus{}, fmt.Errorf("close owned browser: %w", ctxErr)
				}
				if termErr := service.terminateOwned(ctx, state.ChromePID); termErr != nil {
					return BrowserStatus{}, errors.Join(
						fmt.Errorf("close owned browser: %w", closeErr),
						termErr,
					)
				}
			}
		default:
			if termErr := service.terminateOwned(ctx, state.ChromePID); termErr != nil {
				return BrowserStatus{}, termErr
			}
		}
	} else if ok {
		if disconnectErr := disconnectConnection(ctx, connection.Connection); disconnectErr != nil {
			return BrowserStatus{}, fmt.Errorf("disconnect external browser: %w", disconnectErr)
		}
	}
	if err := service.removeState(scope); err != nil {
		return BrowserStatus{}, err
	}
	service.invalidateConnection(scope)
	service.deleteBrowser(scope)
	status := state.status("stopped")
	status.Scope = scope
	return status, nil
}

// Status reports missing, running, or stale state without failing for missing or stale state.
func (service *PersistentService) Status(ctx context.Context, request StatusRequest) (BrowserStatus, error) {
	scope := service.readScope(request.Scope)
	state, err := service.loadState(scope)
	if errors.Is(err, os.ErrNotExist) {
		return BrowserStatus{Status: "missing", Scope: scope}, nil
	}
	if err != nil {
		return BrowserStatus{}, err
	}
	connection, connectErr := service.connectionFor(ctx, scope, state.DebugURL)
	if connectErr != nil {
		service.invalidateConnection(scope)
		return state.status("stale"), nil //nolint:nilerr // stale state is a valid status.
	}
	if pingErr := connection.Ping(ctx); pingErr != nil {
		_ = disconnectConnection(ctx, connection)
		service.invalidateConnection(scope)
		return state.status("stale"), nil //nolint:nilerr // stale state is a valid status.
	}
	return state.status("running"), nil
}

// Connect validates and stores an external browser debugger endpoint.
func (service *PersistentService) Connect(ctx context.Context, request ConnectRequest) (BrowserStatus, error) {
	scope := service.writeScope(request.Scope)
	debugURL, err := service.lookupDebuggerURL(ctx, request.HostPort)
	if err != nil {
		return BrowserStatus{}, err
	}
	connection, err := service.connector.ConnectBrowser(ctx, debugURL)
	if err != nil {
		return BrowserStatus{}, fmt.Errorf("connect external browser: %w", err)
	}
	if oldState, readErr := service.loadState(scope); readErr == nil {
		if closeErr := service.closeStateBrowser(ctx, oldState); closeErr != nil {
			_ = disconnectConnection(ctx, connection)
			return BrowserStatus{}, closeErr
		}
	}
	state := persistentBrowserState{
		Scope:     scope,
		DebugURL:  debugURL,
		Owned:     false,
		UpdatedAt: service.clock().UTC(),
	}
	if err := service.saveState(state); err != nil {
		_ = disconnectConnection(ctx, connection)
		service.invalidateConnection(scope)
		return BrowserStatus{}, err
	}
	service.cacheConnection(scope, debugURL, connection)
	service.resetBrowser(scope, service.clock(), false)
	return state.status("running"), nil
}
