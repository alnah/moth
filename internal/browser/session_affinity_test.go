package browser

import (
	"context"
	"testing"
	"time"
)

func TestPersistentSessionUsesPinnedWorkerWhenOriginalWorkerIsBusy(t *testing.T) {
	ctx := context.Background()
	firstWorker := newSurfaceWorker()
	secondWorker := newSurfaceWorker()
	pool := NewPool(2, WithWorkerFactory(newQueuedGenericWorkerFactory(t, firstWorker, secondWorker)))
	defer func() { _ = pool.Close() }()

	opened := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "work",
		URL:         "https://example.test/one",
	})

	leased, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire(pinned worker) error = %v, want nil", err)
	}
	if leased != firstWorker {
		t.Fatalf("Acquire(pinned worker) = %T, want first session worker", leased)
	}

	pagesResult := make(chan []PageInfo, 1)
	errorResult := make(chan error, 1)
	go func() {
		pages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "work"})
		pagesResult <- pages
		errorResult <- err
	}()

	select {
	case pages := <-pagesResult:
		t.Fatalf("ListPages returned before pinned worker was released: %#v", pages)
	case <-time.After(30 * time.Millisecond):
	}

	pool.Release(leased)
	pages := <-pagesResult
	if err := <-errorResult; err != nil {
		t.Fatalf("ListPages() error = %v, want nil", err)
	}
	assertPageIDs(t, pages, []string{opened.ID})
}

func newQueuedGenericWorkerFactory(t *testing.T, workers ...Worker) WorkerFactory {
	t.Helper()
	created := 0
	return func(context.Context) (Worker, error) {
		if created >= len(workers) {
			t.Fatalf("unexpected worker creation %d", created+1)
		}
		worker := workers[created]
		created++
		return worker, nil
	}
}
