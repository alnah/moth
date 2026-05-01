package browser

import (
	"context"
	"testing"
)

func TestPersistentNamedSessionsManageActivePagesIndependently(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	first := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "work",
		URL:         "https://example.test/one",
	})
	second := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "work",
		URL:         "https://example.test/two",
	})
	other := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "personal",
		SessionName: "work",
		URL:         "https://example.test/private",
	})

	workPages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "work"})
	if err != nil {
		t.Fatalf("ListPages(work) error = %v, want nil", err)
	}
	assertActivePage(t, workPages, second.ID)
	assertPageIDs(t, workPages, []string{first.ID, second.ID})

	personalPages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "personal", SessionName: "work"})
	if err != nil {
		t.Fatalf("ListPages(personal) error = %v, want nil", err)
	}
	assertActivePage(t, personalPages, other.ID)
	assertPageIDs(t, personalPages, []string{other.ID})

	switched, err := pool.SwitchPage(ctx, PageSelection{
		ProfileName: "research",
		SessionName: "work",
		PageID:      first.ID,
	})
	if err != nil {
		t.Fatalf("SwitchPage() error = %v, want nil", err)
	}
	if switched.ID != first.ID || !switched.Active {
		t.Fatalf("SwitchPage() = %#v, want first page active", switched)
	}

	closeErr := pool.ClosePage(ctx, PageSelection{ProfileName: "research", SessionName: "work"})
	if closeErr != nil {
		t.Fatalf("ClosePage(active) error = %v, want nil", closeErr)
	}
	workPages, err = pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "work"})
	if err != nil {
		t.Fatalf("ListPages(work after close) error = %v, want nil", err)
	}
	assertPageIDs(t, workPages, []string{second.ID})
	assertActivePage(t, workPages, second.ID)

	personalPages, err = pool.ListPages(ctx, SessionRequest{ProfileName: "personal", SessionName: "work"})
	if err != nil {
		t.Fatalf("ListPages(personal after work close) error = %v, want nil", err)
	}
	assertPageIDs(t, personalPages, []string{other.ID})
	assertActivePage(t, personalPages, other.ID)
}

func TestInteractiveOperationsUseSelectedActivePageAndStayGeneric(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	first := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "login",
		URL:         "https://example.test/login",
	})
	_ = openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "login",
		URL:         "https://example.test/other",
	})
	if _, err := pool.SwitchPage(ctx, PageSelection{
		ProfileName: "research",
		SessionName: "login",
		PageID:      first.ID,
	}); err != nil {
		t.Fatalf("SwitchPage() error = %v, want nil", err)
	}

	if err := pool.Input(ctx, InputRequest{
		ProfileName: "research",
		SessionName: "login",
		Selector:    `input[name="email"]`,
		Text:        "alexis@example.test",
	}); err != nil {
		t.Fatalf("Input() error = %v, want nil", err)
	}
	if err := pool.Click(ctx, InteractionRequest{
		ProfileName: "research",
		SessionName: "login",
		Selector:    "button[type=submit]",
	}); err != nil {
		t.Fatalf("Click() error = %v, want nil", err)
	}
	if err := pool.Wait(ctx, WaitRequest{
		ProfileName: "research",
		SessionName: "login",
		Selector:    "main.account",
		State:       WaitVisible,
	}); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}

	actions := worker.actions()
	if len(actions) != 3 {
		t.Fatalf("actions len = %d, want 3", len(actions))
	}
	for _, action := range actions {
		if action.PageID != first.ID {
			t.Fatalf("action %#v used page %q, want active page %q", action, action.PageID, first.ID)
		}
	}
	firstActionMatches := actions[0].Kind == "input" &&
		actions[0].Selector == `input[name="email"]` &&
		actions[0].Text == "alexis@example.test"
	if !firstActionMatches {
		t.Fatalf("first action = %#v, want generic input", actions[0])
	}
	if actions[1].Kind != "click" || actions[1].Selector != "button[type=submit]" {
		t.Fatalf("second action = %#v, want generic click", actions[1])
	}
	if actions[2].Kind != "wait" || actions[2].Selector != "main.account" {
		t.Fatalf("third action = %#v, want generic wait", actions[2])
	}
}
