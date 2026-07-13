package browser

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

type poolSession struct {
	worker Worker
	mu     sync.Mutex
}

func sessionKey(profileName string, sessionName string) string {
	return strings.TrimSpace(profileName) + "\x00" + strings.TrimSpace(sessionName)
}

func (pool *Pool) acquireSession(ctx context.Context, key string) (*poolSession, bool, error) {
	pool.mu.Lock()
	session := pool.sessions[key]
	pool.mu.Unlock()
	if session != nil {
		return session, false, nil
	}

	worker, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}

	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		pool.Release(worker)
		return nil, false, ErrPoolClosed
	}
	if session = pool.sessions[key]; session != nil {
		pool.mu.Unlock()
		pool.Release(worker)
		return session, false, nil
	}
	session = &poolSession{worker: worker}
	pool.sessions[key] = session
	pool.mu.Unlock()
	return session, true, nil
}

func (pool *Pool) existingSession(key string) (*poolSession, bool) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	session := pool.sessions[key]
	return session, session != nil
}

func (pool *Pool) acquirePinnedWorker(ctx context.Context, worker Worker) (Worker, error) {
	for {
		acquired, waitCh, err := pool.reservePinnedWorker(worker)
		if err != nil || acquired != nil {
			return acquired, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pool.closedCh:
			return nil, ErrPoolClosed
		case <-waitCh:
		}
	}
}

func (pool *Pool) reservePinnedWorker(worker Worker) (Worker, <-chan struct{}, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.closed {
		return nil, nil, ErrPoolClosed
	}
	for index, availableWorker := range pool.available {
		if availableWorker == worker {
			pool.available = append(pool.available[:index], pool.available[index+1:]...)
			return availableWorker, nil, nil
		}
	}
	if !pool.knowsWorkerLocked(worker) {
		return nil, nil, errors.New("browser session worker is not in pool")
	}
	return nil, pool.waitCh, nil
}

func (pool *Pool) knowsWorkerLocked(worker Worker) bool {
	return slices.Contains(pool.workers, worker)
}

func (pool *Pool) removeSessionIfEmpty(ctx context.Context, key string, session *poolSession) error {
	persistentWorker, err := requirePersistentPageWorker(session.worker)
	if err != nil {
		return err
	}
	profileName, sessionName := splitSessionKey(key)
	pages, err := persistentWorker.ListPersistentPages(ctx, SessionRequest{
		ProfileName: profileName,
		SessionName: sessionName,
	})
	if err != nil {
		return err
	}
	if len(pages) != 0 {
		return nil
	}

	pool.removeSession(key, session)
	return nil
}

func (pool *Pool) removeSession(key string, session *poolSession) {
	pool.mu.Lock()
	if pool.sessions[key] == session {
		delete(pool.sessions, key)
	}
	pool.mu.Unlock()
}

func splitSessionKey(key string) (string, string) {
	profileName, sessionName, found := strings.Cut(key, "\x00")
	if !found {
		return key, ""
	}
	return profileName, sessionName
}

func (pool *Pool) withSessionWorker(
	ctx context.Context,
	request SessionRequest,
	create bool,
	use func(Worker) error,
) error {
	key := sessionKey(request.ProfileName, request.SessionName)
	session, created, err := pool.sessionForOperation(ctx, key, create)
	if err != nil {
		return err
	}
	if session == nil {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()

	if created {
		defer pool.Release(session.worker)
		operationErr := use(session.worker)
		if operationErr != nil {
			pool.removeSession(key, session)
			return operationErr
		}
		return nil
	}

	worker, err := pool.acquirePinnedWorker(ctx, session.worker)
	if err != nil {
		return err
	}
	defer pool.Release(worker)
	return use(worker)
}

func (pool *Pool) sessionForOperation(ctx context.Context, key string, create bool) (*poolSession, bool, error) {
	if create {
		return pool.acquireSession(ctx, key)
	}
	session, ok := pool.existingSession(key)
	if !ok {
		return nil, false, fmt.Errorf("browser session %q not found", key)
	}
	return session, false, nil
}
