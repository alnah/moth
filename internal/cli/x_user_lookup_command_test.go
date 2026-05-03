package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/content"
	xclient "github.com/alnah/moth/internal/x"
)

var _ interface {
	LookupUserByUsername(context.Context, xclient.UsernameLookupOptions) (content.Pack, error)
} = (XService)(nil)

func TestXUserLookupCommandRoutesUsernameAndRendersContentPack(t *testing.T) {
	harness := newCommandHarness()

	stdout, stderr, err := harness.execute("x", "user-lookup", "alnah")
	if err != nil {
		t.Fatalf("execute x user-lookup: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)

	want := xclient.UsernameLookupOptions{Username: "alnah"}
	if !reflect.DeepEqual(harness.x.usernameLookup, want) {
		t.Fatalf("x user-lookup options = %#v, want %#v", harness.x.usernameLookup, want)
	}
	if !harness.x.usernameLookupCalled {
		t.Fatal("x user-lookup did not call username lookup service")
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}
