package browser

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolvePoolSizeClampsToMothBrowserBounds(t *testing.T) {
	previousMaxProcs := runtime.GOMAXPROCS(16)
	defer runtime.GOMAXPROCS(previousMaxProcs)

	for _, tc := range []struct {
		name    string
		workers int
		want    int
	}{
		{name: "negative explicit value uses minimum", workers: -3, want: 1},
		{name: "automatic value is clamped to maximum", workers: 0, want: 4},
		{name: "minimum explicit value", workers: 1, want: 1},
		{name: "maximum explicit value", workers: 4, want: 4},
		{name: "oversized explicit value is clamped", workers: 99, want: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePoolSize(tc.workers)
			if got != tc.want {
				t.Fatalf("ResolvePoolSize(%d) = %d, want %d", tc.workers, got, tc.want)
			}
		})
	}
}

func TestPoolCreatesWorkersLazilyAndReusesReleasedWorkers(t *testing.T) {
	var created atomic.Int32
	worker := &fakeWorker{}
	pool := NewPool(2, WithWorkerFactory(func(context.Context) (Worker, error) {
		created.Add(1)
		return worker, nil
	}))
	defer func() { _ = pool.Close() }()

	if got := created.Load(); got != 0 {
		t.Fatalf("workers created before first acquire = %d, want 0", got)
	}

	first, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(first) error = %v, want nil", err)
	}
	if got := created.Load(); got != 1 {
		t.Fatalf("workers created after first acquire = %d, want 1", got)
	}
	pool.Release(first)

	second, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(second) error = %v, want nil", err)
	}
	pool.Release(second)

	if first != second {
		t.Fatal("Acquire after Release returned a different worker, want released worker reused")
	}
	if got := created.Load(); got != 1 {
		t.Fatalf("workers created after reuse = %d, want 1", got)
	}
}

func TestAcquireCanBeCancelledWhileWaitingForWorker(t *testing.T) {
	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{})))
	defer func() { _ = pool.Close() }()

	leased, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(initial) error = %v, want nil", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = pool.Acquire(ctx)
	if err == nil {
		t.Fatal("Acquire(waiting) error = nil, want context deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire(waiting) error = %v, want context deadline", err)
	}

	pool.Release(leased)
}

func TestReleaseAfterCloseIsSafe(t *testing.T) {
	var closed atomic.Int32
	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{
		close: func() error {
			closed.Add(1)
			return nil
		},
	})))

	leased, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v, want nil", err)
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	pool.Release(leased)

	if err := pool.Close(); err != nil {
		t.Fatalf("Close() after Release() error = %v, want nil", err)
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("worker close count = %d, want 1", got)
	}
}

func TestAcquireAfterCloseReturnsPoolClosedWhenReleasedWorkerWasAvailable(t *testing.T) {
	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{})))

	leased, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(initial) error = %v, want nil", err)
	}
	pool.Release(leased)

	closeErr := pool.Close()
	if closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}

	_, err = pool.Acquire(context.Background())
	if !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("Acquire(after close) error = %v, want ErrPoolClosed", err)
	}
}

func TestCloseIsIdempotentAndJoinsWorkerErrors(t *testing.T) {
	firstErr := errors.New("close first browser")
	secondErr := errors.New("close second browser")
	var firstClosed atomic.Int32
	var secondClosed atomic.Int32

	pool := NewPool(2, WithWorkerFactory(newQueuedWorkerFactory(t,
		&fakeWorker{close: func() error {
			firstClosed.Add(1)
			return firstErr
		}},
		&fakeWorker{close: func() error {
			secondClosed.Add(1)
			return secondErr
		}},
	)))

	first, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(first) error = %v, want nil", err)
	}
	second, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(second) error = %v, want nil", err)
	}
	pool.Release(first)
	pool.Release(second)

	err = pool.Close()
	if !errors.Is(err, firstErr) {
		t.Fatalf("Close() error = %v, want joined first error", err)
	}
	if !errors.Is(err, secondErr) {
		t.Fatalf("Close() error = %v, want joined second error", err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Close() second call error = %v, want nil", err)
	}
	if got := firstClosed.Load(); got != 1 {
		t.Fatalf("first worker close count = %d, want 1", got)
	}
	if got := secondClosed.Load(); got != 1 {
		t.Fatalf("second worker close count = %d, want 1", got)
	}
}
