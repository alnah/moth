//go:build browser

package browser

import (
	"net/http"
	"testing"
	"time"

	"github.com/alnah/moth/internal/content"
)

func TestRodPoolPersistentSessionSwitchCloseAndChallenge(t *testing.T) {
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/one":
			_, _ = w.Write([]byte(`<!doctype html><title>First live page</title><h1>First</h1>`))
		case "/two":
			_, _ = w.Write([]byte(`<!doctype html><title>Second live page</title><h1>Second</h1>`))
		case "/captcha":
			_, _ = w.Write([]byte(`<!doctype html><title>Check</title><main>Please verify you are human. CAPTCHA required.</main>`))
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 20*time.Second)

	first, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "live-session",
		URL:         server.URL + "/one",
	})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("OpenPage(first real Rod page) error = %v, want nil", err)
	}
	if first.ID == "" || first.Title != "First live page" || !first.Active {
		t.Fatalf("OpenPage(first real Rod page) = %#v, want ID, title, active", first)
	}

	second, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "live-session",
		URL:         server.URL + "/two",
	})
	if err != nil {
		t.Fatalf("OpenPage(second real Rod page) error = %v, want nil", err)
	}
	if second.ID == "" || second.ID == first.ID || second.Title != "Second live page" || !second.Active {
		t.Fatalf("OpenPage(second real Rod page) = %#v, want distinct active page", second)
	}

	pages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "live-session"})
	if err != nil {
		t.Fatalf("ListPages(real Rod session) error = %v, want nil", err)
	}
	assertPageIDs(t, pages, []string{first.ID, second.ID})
	assertActivePage(t, pages, second.ID)

	switched, err := pool.SwitchPage(ctx, PageSelection{
		ProfileName: "research",
		SessionName: "live-session",
		PageID:      first.ID,
	})
	if err != nil {
		t.Fatalf("SwitchPage(real Rod session) error = %v, want nil", err)
	}
	if switched.ID != first.ID || !switched.Active {
		t.Fatalf("SwitchPage(real Rod session) = %#v, want first page active", switched)
	}
	pages, err = pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "live-session"})
	if err != nil {
		t.Fatalf("ListPages(after switch) error = %v, want nil", err)
	}
	assertActivePage(t, pages, first.ID)

	if err := pool.ClosePage(ctx, PageSelection{ProfileName: "research", SessionName: "live-session", PageID: second.ID}); err != nil {
		t.Fatalf("ClosePage(second real Rod page) error = %v, want nil", err)
	}
	pages, err = pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "live-session"})
	if err != nil {
		t.Fatalf("ListPages(after closing second page) error = %v, want nil", err)
	}
	assertPageIDs(t, pages, []string{first.ID})
	assertActivePage(t, pages, first.ID)

	captchaPage, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "challenge",
		URL:         server.URL + "/captcha",
	})
	if err != nil {
		t.Fatalf("OpenPage(captcha real Rod page) error = %v, want nil", err)
	}
	challenge, err := pool.DetectManualChallenge(ctx, ManualChallengeRequest{
		ProfileName: "research",
		SessionName: "challenge",
		PageID:      captchaPage.ID,
	})
	if err != nil {
		t.Fatalf("DetectManualChallenge(real Rod page) error = %v, want nil", err)
	}
	if !challenge.ManualRequired || challenge.Kind != "captcha" || challenge.Solved {
		t.Fatalf("DetectManualChallenge(real Rod page) = %#v, want unsolved captcha required", challenge)
	}
	if !hasWarning(challenge.Warnings, content.WarningCaptchaPossible) {
		t.Fatalf("DetectManualChallenge(real Rod page) warnings = %#v, want captcha_possible", challenge.Warnings)
	}

	if err := pool.ClosePage(ctx, PageSelection{ProfileName: "research", SessionName: "live-session", PageID: first.ID}); err != nil {
		t.Fatalf("ClosePage(last real Rod page) error = %v, want nil", err)
	}
	pages, err = pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "live-session"})
	if err != nil {
		t.Fatalf("ListPages(after closing last page) error = %v, want nil", err)
	}
	if len(pages) != 0 {
		t.Fatalf("ListPages(after closing last page) = %#v, want empty", pages)
	}
}

func TestRodPoolInputsClicksWaitsAndReadsAccessibilityTree(t *testing.T) {
	submissions := make(chan string, 1)
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/form":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<title>Interactive form</title>
<label for="query">Query</label>
<input id="query" name="query">
<button id="submit" onclick="submitQuery()">Send search</button>
<p id="done" style="display:none">Submitted</p>
<script>
async function submitQuery() {
  const value = document.querySelector('#query').value;
  await fetch('/submitted?value=' + encodeURIComponent(value));
  document.querySelector('#done').style.display = 'block';
}
</script>`))
		case "/submitted":
			select {
			case submissions <- r.URL.Query().Get("value"):
			default:
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 20*time.Second)

	page, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "interactions",
		URL:         server.URL + "/form",
	})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("OpenPage(form real Rod page) error = %v, want nil", err)
	}

	if err := pool.Input(ctx, InputRequest{
		ProfileName: "research",
		SessionName: "interactions",
		PageID:      page.ID,
		Selector:    "#query",
		Text:        "moth browser",
	}); err != nil {
		t.Fatalf("Input(real Rod page) error = %v, want nil", err)
	}
	if err := pool.Click(ctx, InteractionRequest{
		ProfileName: "research",
		SessionName: "interactions",
		PageID:      page.ID,
		Selector:    "#submit",
	}); err != nil {
		t.Fatalf("Click(real Rod page) error = %v, want nil", err)
	}
	if err := pool.Wait(ctx, WaitRequest{
		ProfileName: "research",
		SessionName: "interactions",
		PageID:      page.ID,
		Selector:    "#done",
		State:       WaitVisible,
	}); err != nil {
		t.Fatalf("Wait(real Rod page) error = %v, want nil", err)
	}

	select {
	case got := <-submissions:
		if got != "moth browser" {
			t.Fatalf("submitted value = %q, want moth browser", got)
		}
	case <-ctx.Done():
		t.Fatalf("submitted value not received before context done: %v", ctx.Err())
	}

	tree, err := pool.AccessibilityTree(ctx, AccessibilityRequest{
		ProfileName: "research",
		SessionName: "interactions",
		PageID:      page.ID,
		MaxDepth:    4,
	})
	if err != nil {
		t.Fatalf("AccessibilityTree(real Rod page) error = %v, want nil", err)
	}
	if !accessibilityTreeHasName(tree, "Send search") {
		t.Fatalf("AccessibilityTree(real Rod page) = %#v, want Send search node", tree)
	}
}

func TestRodPoolWaitAttachedFindsHiddenElement(t *testing.T) {
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/attach":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<title>Attach page</title>
<button id="attach" onclick="attachHidden()">Attach hidden marker</button>
<script>
function attachHidden() {
  const marker = document.createElement('p');
  marker.id = 'attached';
  marker.hidden = true;
  marker.textContent = 'attached but hidden';
  document.body.appendChild(marker);
}
</script>`))
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 20*time.Second)

	page, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "attached-wait",
		URL:         server.URL + "/attach",
	})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("OpenPage(attach page) error = %v, want nil", err)
	}
	if err := pool.Click(ctx, InteractionRequest{
		ProfileName: "research",
		SessionName: "attached-wait",
		PageID:      page.ID,
		Selector:    "#attach",
	}); err != nil {
		t.Fatalf("Click(attach hidden marker) error = %v, want nil", err)
	}
	if err := pool.Wait(ctx, WaitRequest{
		ProfileName: "research",
		SessionName: "attached-wait",
		PageID:      page.ID,
		Selector:    "#attached",
		State:       WaitAttached,
	}); err != nil {
		t.Fatalf("Wait(attached hidden marker) error = %v, want nil", err)
	}
}

func TestRodPoolDetectManualChallengeIgnoresNormalPage(t *testing.T) {
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/normal":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><title>Normal page</title><main>Research notes without a challenge.</main>`))
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 15*time.Second)

	page, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "normal-challenge",
		URL:         server.URL + "/normal",
	})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("OpenPage(normal page) error = %v, want nil", err)
	}
	got, err := pool.DetectManualChallenge(ctx, ManualChallengeRequest{
		ProfileName: "research",
		SessionName: "normal-challenge",
		PageID:      page.ID,
	})
	if err != nil {
		t.Fatalf("DetectManualChallenge(normal page) error = %v, want nil", err)
	}
	if got.ManualRequired || got.Solved {
		t.Fatalf("DetectManualChallenge(normal page) = %#v, want no manual challenge and unsolved", got)
	}
	if hasWarning(got.Warnings, content.WarningCaptchaPossible) {
		t.Fatalf("DetectManualChallenge(normal page) warnings = %#v, want no captcha_possible", got.Warnings)
	}
}

func accessibilityTreeHasName(tree AccessibilityTree, name string) bool {
	for _, node := range tree.Nodes {
		if node.Name == name {
			return true
		}
	}
	return false
}
