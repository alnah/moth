package supervisor_test

import (
	"context"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

func TestServerIdleAfterTimeout(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	server := supervisor.NewServer(supervisor.ServerOptions{
		Version: "test-version",
		Clock:   func() time.Time { return now },
	})

	if server.Idle(5 * time.Minute) {
		t.Fatal("Idle() at startup = true, want false")
	}
	now = now.Add(6 * time.Minute)
	if !server.Idle(5 * time.Minute) {
		t.Fatal("Idle() after timeout = false, want true")
	}
}

func TestServerReapWithoutServices(t *testing.T) {
	server := supervisor.NewServer(supervisor.ServerOptions{Version: "test-version"})

	report, err := server.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap() error = %v, want nil", err)
	}
	if report.ClosedPages != 0 || report.ClosedSessions != 0 || report.ClosedBrowsers != 0 {
		t.Fatalf("Reap() report = %#v, want zero counts", report)
	}
}
