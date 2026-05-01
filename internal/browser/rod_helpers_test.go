package browser

import (
	"reflect"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func TestBlockedURLPatternsIncludeConfiguredResourceClasses(t *testing.T) {
	got := blockedURLPatterns(ResourceSet(ResourceImages) | ResourceSet(ResourceFonts) | ResourceSet(ResourceMedia))
	want := []string{
		"*.avif", "*.gif", "*.jpeg", "*.jpg", "*.png", "*.svg", "*.webp",
		"*.otf", "*.ttf", "*.woff", "*.woff2",
		"*.avi", "*.m4a", "*.mp3", "*.mp4", "*.mpeg", "*.ogg", "*.webm",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blocked URL patterns = %#v, want %#v", got, want)
	}
	if got := blockedURLPatterns(0); len(got) != 0 {
		t.Fatalf("blocked URL patterns without resources = %#v, want empty", got)
	}
}

func TestSelectedRodPageIndexSelectsActiveExplicitAndReportsMissing(t *testing.T) {
	session := &rodSession{
		pages: []*rodPersistentPage{
			{id: "first"},
			{id: "second"},
		},
		active: 1,
	}

	index, err := selectedRodPageIndex(session, "")
	if err != nil || index != 1 {
		t.Fatalf("selected active index = %d, %v; want 1, nil", index, err)
	}
	index, err = selectedRodPageIndex(session, "first")
	if err != nil || index != 0 {
		t.Fatalf("selected explicit index = %d, %v; want 0, nil", index, err)
	}
	if _, err := selectedRodPageIndex(session, "missing"); err == nil {
		t.Fatal("selected missing page error = nil, want error")
	}
	if _, err := selectedRodPageIndex(&rodSession{active: -1}, ""); err == nil {
		t.Fatal("selected active page without pages error = nil, want error")
	}
}

func TestRodSessionLockedReusesNamedSession(t *testing.T) {
	worker := &rodWorker{sessions: make(map[string]*rodSession)}
	first := worker.sessionLocked("profile", "work")
	second := worker.sessionLocked("profile", "work")
	other := worker.sessionLocked("profile", "other")

	if first != second {
		t.Fatal("sessionLocked returned different sessions for same profile/session")
	}
	if first == other {
		t.Fatal("sessionLocked reused session for different session name")
	}
	if first.active != -1 || other.active != -1 {
		t.Fatalf("new session active indices = %d/%d, want -1/-1", first.active, other.active)
	}
}

func TestAXValueStringHandlesNilAndStringValues(t *testing.T) {
	if got := axValueString(nil); got != "" {
		t.Fatalf("nil AX value = %q, want empty", got)
	}
	got := axValueString(&proto.AccessibilityAXValue{Value: gson.New("Pay now")})
	if got != "Pay now" {
		t.Fatalf("AX value = %q, want Pay now", got)
	}
}
