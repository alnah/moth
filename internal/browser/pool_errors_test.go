package browser

import (
	"context"
	"errors"
	"testing"
)

func TestAcquireReleasesReservedSlotWhenFactoryFails(t *testing.T) {
	factoryErr := errors.New("factory failed")
	pool := NewPool(1, WithWorkerFactory(func(context.Context) (Worker, error) {
		return nil, factoryErr
	}))
	defer func() { _ = pool.Close() }()

	_, err := pool.Acquire(context.Background())
	if !errors.Is(err, factoryErr) {
		t.Fatalf("Acquire(factory error) error = %v, want factory error", err)
	}
	if pool.created != 0 {
		t.Fatalf("created slots after factory error = %d, want 0", pool.created)
	}
}

func TestAcquireRejectsNilFactoryWorker(t *testing.T) {
	pool := NewPool(1, WithWorkerFactory(nilWorkerFactory))
	defer func() { _ = pool.Close() }()

	_, err := pool.Acquire(context.Background())
	if err == nil {
		t.Fatal("Acquire(nil worker) error = nil, want error")
	}
	if pool.created != 0 {
		t.Fatalf("created slots after nil worker = %d, want 0", pool.created)
	}
}

func nilWorkerFactory(context.Context) (Worker, error) {
	var worker Worker
	return worker, nil
}

func TestDefaultRodFactoryReportsInvalidBrowserBinary(t *testing.T) {
	pool := NewPool(1, WithBrowserBin(t.TempDir()))
	defer func() { _ = pool.Close() }()

	_, err := pool.Acquire(context.Background())
	if !errors.Is(err, ErrBrowserMissing) {
		t.Fatalf("Acquire(invalid browser bin) error = %v, want ErrBrowserMissing", err)
	}
}
