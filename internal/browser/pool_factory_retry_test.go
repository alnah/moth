package browser

import (
	"context"
	"errors"
	"testing"
)

func TestAcquireCanCreateWorkerAfterFactoryFailure(t *testing.T) {
	factoryErr := errors.New("temporary factory failure")
	created := 0
	pool := NewPool(1, WithWorkerFactory(func(context.Context) (Worker, error) {
		created++
		if created == 1 {
			return nil, factoryErr
		}
		return &fakeWorker{}, nil
	}))
	defer func() { _ = pool.Close() }()

	_, err := pool.Acquire(context.Background())
	if !errors.Is(err, factoryErr) {
		t.Fatalf("Acquire(first factory call) error = %v, want %v", err, factoryErr)
	}
	worker, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(after factory failure) error = %v, want nil", err)
	}
	if worker == nil {
		t.Fatal("Acquire(after factory failure) worker = nil, want worker")
	}
}
