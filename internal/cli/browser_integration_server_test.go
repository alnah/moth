//go:build browser

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type slowPageServer struct {
	Server    *httptest.Server
	Requested chan struct{}
	Release   chan struct{}

	once sync.Once
}

func newSlowPageServer(t *testing.T) *slowPageServer {
	t.Helper()

	server := &slowPageServer{
		Requested: make(chan struct{}),
		Release:   make(chan struct{}),
	}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slow" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<!doctype html><title>slow integration page</title>"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		server.once.Do(func() { close(server.Requested) })

		select {
		case <-server.Release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.close)
	return server
}

func (server *slowPageServer) close() {
	select {
	case <-server.Release:
	default:
		close(server.Release)
	}
	server.Server.Close()
}
