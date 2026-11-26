package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Shutdown releases cached browsers without touching external browser processes.
func (service *PersistentService) Shutdown(ctx context.Context) error {
	var errs []error
	for _, scope := range []string{"local", "global"} {
		state, err := service.loadState(scope)
		if errors.Is(err, os.ErrNotExist) {
			service.invalidateConnection(scope)
			service.deleteBrowser(scope)
			continue
		}
		if err != nil {
			service.invalidateConnection(scope)
			service.deleteBrowser(scope)
			continue
		}

		closeErr := service.closeStateBrowser(ctx, state)
		if closeErr != nil {
			errs = append(errs, fmt.Errorf("shutdown %s browser: %w", scope, closeErr))
		} else if state.Owned {
			if removeErr := service.removeState(scope); removeErr != nil {
				errs = append(errs, removeErr)
			}
		}
		service.invalidateConnection(scope)
		service.deleteBrowser(scope)
	}
	return errors.Join(errs...)
}

func (service *PersistentService) closeStateBrowser(
	ctx context.Context,
	state persistentBrowserState,
) error {
	connection, ok := service.cachedConnection(state.Scope)
	if !ok || connection.DebugURL != state.DebugURL {
		connected, connectErr := service.connectionFor(ctx, state.Scope, state.DebugURL)
		if connectErr != nil {
			if state.Owned {
				return service.terminateOwned(ctx, state.ChromePID)
			}
			return nil
		}
		connection = cachedBrowserConnection{DebugURL: state.DebugURL, Connection: connected}
		ok = true
	}
	if !ok {
		if state.Owned {
			return service.terminateOwned(ctx, state.ChromePID)
		}
		return nil
	}
	if !state.Owned {
		return disconnectConnection(ctx, connection.Connection)
	}
	closeErr := connection.Connection.Close(ctx)
	if closeErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return closeErr
	}
	termErr := service.terminateOwned(ctx, state.ChromePID)
	if termErr != nil {
		return errors.Join(closeErr, termErr)
	}
	return nil
}

func (service *PersistentService) terminateOwned(ctx context.Context, pid int) error {
	if service.terminateProcess == nil || pid <= 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := service.terminateProcess(pid); err != nil {
		return fmt.Errorf("terminate browser pid %d: %w", pid, err)
	}
	return nil
}
