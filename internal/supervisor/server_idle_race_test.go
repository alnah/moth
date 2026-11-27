package supervisor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServerCloseCancelsAcceptedCall(t *testing.T) {
	now := time.Now()
	server := NewServer(ServerOptions{
		Version: "test-version",
		Token:   "test-token",
		Clock:   func() time.Time { return now },
	})
	server.lastActivity = now.Add(-time.Hour)
	if !server.Idle(time.Minute) {
		t.Fatal("Idle() = false, want true before the call")
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCall := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCall)
	server.handlers["test.block"] = func(
		ctx context.Context,
		_ requestMetadata,
		_ json.RawMessage,
	) (json.RawMessage, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return nil, nil
		}
	}

	request := httptest.NewRequest(
		http.MethodPost,
		CallPath,
		strings.NewReader(`{"method":"test.block"}`),
	)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		server.handleCall(recorder, request)
	}()
	<-started

	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	select {
	case <-callDone:
	case <-time.After(250 * time.Millisecond):
		releaseCall()
		<-callDone
		t.Fatal("accepted call was not canceled by Server.Close")
	}
}
