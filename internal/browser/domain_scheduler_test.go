package browser

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFetchPageSerializesSameRegistrableDomain(t *testing.T) {
	const firstURL = "https://news.example.test/one"
	const secondURL = "https://shop.example.test/two"

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	worker := &fakeWorker{openPage: func(ctx context.Context, request PageRequest) (LoadedPage, error) {
		switch request.URL {
		case firstURL:
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return LoadedPage{}, ctx.Err()
			}
		case secondURL:
			close(secondStarted)
		}
		return LoadedPage{URL: request.URL, HTML: "<html><title>ok</title><body>ok</body></html>"}, nil
	}}
	pool := NewPool(2, WithWorkerFactory(newQueuedWorkerFactory(t, worker, worker)))
	defer func() { _ = pool.Close() }()

	firstDone := make(chan error, 1)
	go func() {
		_, err := pool.FetchPage(context.Background(), PageRequest{URL: firstURL})
		firstDone <- err
	}()
	waitForClosed(t, firstStarted, "first same-domain fetch to start")

	secondDone := make(chan error, 1)
	go func() {
		_, err := pool.FetchPage(context.Background(), PageRequest{URL: secondURL})
		secondDone <- err
	}()

	select {
	case <-secondStarted:
		close(releaseFirst)
		t.Fatal("second same-registrable-domain fetch started while first fetch was active")
	case <-time.After(30 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first FetchPage() error = %v, want nil", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second FetchPage() error = %v, want nil", err)
	}
	waitForClosed(t, secondStarted, "second same-domain fetch to start after first finished")
}

func TestFetchPageAllowsDifferentRegistrableDomainsConcurrently(t *testing.T) {
	const firstURL = "https://example.test/one"
	const secondURL = "https://example.invalid/two"

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseBoth := make(chan struct{})

	worker := &fakeWorker{openPage: func(ctx context.Context, request PageRequest) (LoadedPage, error) {
		switch request.URL {
		case firstURL:
			close(firstStarted)
		case secondURL:
			close(secondStarted)
		}
		select {
		case <-releaseBoth:
		case <-ctx.Done():
			return LoadedPage{}, ctx.Err()
		}
		return LoadedPage{URL: request.URL, HTML: "<html><title>ok</title><body>ok</body></html>"}, nil
	}}
	pool := NewPool(2, WithWorkerFactory(newQueuedWorkerFactory(t, worker, worker)))
	defer func() { _ = pool.Close() }()

	firstDone := make(chan error, 1)
	go func() {
		_, err := pool.FetchPage(context.Background(), PageRequest{URL: firstURL})
		firstDone <- err
	}()
	waitForClosed(t, firstStarted, "first cross-domain fetch to start")

	secondDone := make(chan error, 1)
	go func() {
		_, err := pool.FetchPage(context.Background(), PageRequest{URL: secondURL})
		secondDone <- err
	}()
	waitForClosed(t, secondStarted, "second cross-domain fetch to start while first is active")

	close(releaseBoth)
	if err := <-firstDone; err != nil {
		t.Fatalf("first FetchPage() error = %v, want nil", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second FetchPage() error = %v, want nil", err)
	}
}

func TestFetchPageWaitingForSameDomainHonorsContextCancellation(t *testing.T) {
	const firstURL = "https://docs.example.test/one"
	const secondURL = "https://cdn.example.test/two"

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	worker := &fakeWorker{openPage: func(ctx context.Context, request PageRequest) (LoadedPage, error) {
		switch request.URL {
		case firstURL:
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return LoadedPage{}, ctx.Err()
			}
		case secondURL:
			close(secondStarted)
		}
		return LoadedPage{URL: request.URL, HTML: "<html><title>ok</title><body>ok</body></html>"}, nil
	}}
	pool := NewPool(2, WithWorkerFactory(newQueuedWorkerFactory(t, worker, worker)))
	defer func() { _ = pool.Close() }()

	firstDone := make(chan error, 1)
	go func() {
		_, err := pool.FetchPage(context.Background(), PageRequest{URL: firstURL})
		firstDone <- err
	}()
	waitForClosed(t, firstStarted, "first same-domain fetch to start")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := pool.FetchPage(ctx, PageRequest{URL: secondURL})
	if err == nil {
		t.Fatal("FetchPage(waiting on same domain) error = nil, want context deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FetchPage(waiting on same domain) error = %v, want context deadline", err)
	}

	select {
	case <-secondStarted:
		t.Fatal("second same-domain fetch started after its context was cancelled")
	default:
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first FetchPage() error = %v, want nil", err)
	}
}

func waitForClosed(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", description)
	}
}
