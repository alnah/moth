package browser

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestOpenPageFailureDoesNotRetainPersistentSession(t *testing.T) {
	ctx := context.Background()
	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{})))
	defer func() { _ = pool.Close() }()

	_, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "failed-open",
		URL:         "https://example.test",
	})
	if err == nil || !strings.Contains(err.Error(), "persistent pages") {
		t.Fatalf("OpenPage(unsupported worker) error = %v, want persistent pages context", err)
	}

	pages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "failed-open"})
	if err != nil {
		t.Fatalf("ListPages(after failed OpenPage) error = %v, want nil", err)
	}
	if len(pages) != 0 {
		t.Fatalf("ListPages(after failed OpenPage) = %#v, want empty", pages)
	}
}

func TestPersistentPageOpenErrorDoesNotRetainSession(t *testing.T) {
	ctx := context.Background()
	openErr := errors.New("open persistent page failed")
	worker := &persistentOnlyWorker{openErr: openErr}
	pool := NewPool(1, WithWorkerFactory(func(context.Context) (Worker, error) { return worker, nil }))
	defer func() { _ = pool.Close() }()

	_, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "failed-open",
		URL:         "https://example.test",
	})
	if !errors.Is(err, openErr) {
		t.Fatalf("OpenPage(worker failure) error = %v, want %v", err, openErr)
	}

	_, err = pool.SwitchPage(ctx, PageSelection{ProfileName: "research", SessionName: "failed-open"})
	if err == nil || !strings.Contains(err.Error(), "browser session") {
		t.Fatalf("SwitchPage(after failed OpenPage) error = %v, want missing session context", err)
	}
}
