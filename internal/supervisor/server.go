package supervisor

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alnah/moth/internal/browser"
)

type methodHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

// Server serves the daemon protocol.
type Server struct {
	version   string
	token     string
	startedAt time.Time
	clock     func() time.Time
	lifecycle browser.LifecycleOptions
	logger    *slog.Logger
	pool      *browser.Pool
	handlers  map[string]methodHandler

	activityMu   sync.Mutex
	lastActivity time.Time
	activeCalls  atomic.Int64

	shutdownOnce sync.Once
	shutdownCh   chan struct{}
}

// NewServer creates a daemon protocol server.
func NewServer(options ServerOptions) *Server {
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	startedAt := options.StartedAt
	if startedAt.IsZero() {
		startedAt = clock()
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	server := &Server{
		version:      options.Version,
		token:        options.Token,
		startedAt:    startedAt,
		clock:        clock,
		lifecycle:    options.Lifecycle,
		logger:       logger,
		pool:         options.Pool,
		handlers:     make(map[string]methodHandler),
		lastActivity: startedAt,
		shutdownCh:   make(chan struct{}),
	}
	server.handlers[MethodHealth] = server.handleHealth
	server.handlers[MethodShutdown] = server.handleShutdown
	server.handlers[MethodFetchRendered] = server.handleFetchRendered
	return server
}

// Reap runs one persistent browser lifecycle pass.
func (server *Server) Reap(context.Context) (browser.ReapReport, error) {
	return browser.ReapReport{}, nil
}

// Idle reports whether the daemon can stop after the given inactivity.
func (server *Server) Idle(timeout time.Duration) bool {
	if server.activeCalls.Load() != 0 {
		return false
	}
	server.activityMu.Lock()
	lastActivity := server.lastActivity
	server.activityMu.Unlock()
	if server.clock().Sub(lastActivity) < timeout {
		return false
	}
	return server.activeBrowsers() == 0
}

// Handler returns the daemon HTTP handler.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(CallPath, server.handleCall)
	return mux
}

// Shutdown requests a graceful daemon stop.
func (server *Server) Shutdown(context.Context) error {
	server.requestShutdown()
	return nil
}

// Close releases browser resources owned by the server.
func (server *Server) Close() error {
	if server.pool == nil {
		return nil
	}
	return server.pool.Close()
}

func (server *Server) done() <-chan struct{} {
	return server.shutdownCh
}

func (server *Server) requestShutdown() {
	server.shutdownOnce.Do(func() {
		close(server.shutdownCh)
	})
}

func (server *Server) handleCall(writer http.ResponseWriter, request *http.Request) {
	server.beginCall()
	defer server.endCall()
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !server.authorized(request) {
		writeReply(writer, http.StatusUnauthorized, Reply{
			Error: &CallError{
				Code:    CodeUnauthorized,
				Message: "invalid daemon token",
			},
		})
		return
	}

	body := http.MaxBytesReader(writer, request.Body, maxRequestBody)
	defer func() { _ = body.Close() }()

	var call Call
	if err := json.NewDecoder(body).Decode(&call); err != nil {
		status := http.StatusBadRequest
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			status = http.StatusRequestEntityTooLarge
		}
		writeReply(writer, status, Reply{
			Error: &CallError{
				Code:    CodeProtocolError,
				Message: err.Error(),
			},
		})
		return
	}
	if strings.TrimSpace(call.Method) == "" {
		writeReply(writer, http.StatusBadRequest, Reply{
			Error: &CallError{
				Code:    CodeProtocolError,
				Message: "missing daemon method",
			},
		})
		return
	}

	result, err := server.dispatch(request.Context(), call)
	if err != nil {
		code := CodeMethodFailed
		switch {
		case errors.Is(err, errMethodNotFound):
			code = CodeMethodNotFound
		case errors.Is(err, errInvalidParams):
			code = CodeProtocolError
		}
		writeReply(writer, http.StatusOK, Reply{
			Error: &CallError{
				Code:    code,
				Message: err.Error(),
			},
		})
		return
	}
	writeReply(writer, http.StatusOK, Reply{Result: result})
}

func (server *Server) authorized(request *http.Request) bool {
	if server.token == "" {
		return false
	}
	const prefix = "Bearer "
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return subtle.ConstantTimeCompare([]byte(token), []byte(server.token)) == 1
}

func (server *Server) dispatch(ctx context.Context, call Call) (json.RawMessage, error) {
	handler, ok := server.handlers[call.Method]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errMethodNotFound, call.Method)
	}
	return handler(ctx, call.Params)
}

func (server *Server) handleHealth(context.Context, json.RawMessage) (json.RawMessage, error) {
	return marshalResult(server.health())
}

func (server *Server) handleShutdown(context.Context, json.RawMessage) (json.RawMessage, error) {
	server.requestShutdown()
	return marshalResult(server.health())
}

func (server *Server) handleFetchRendered(
	ctx context.Context,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params FetchRenderedParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.URL) == "" {
		return nil, fmt.Errorf("%w: url is required", errInvalidParams)
	}
	if server.pool == nil {
		return nil, errors.New("browser pool unavailable")
	}

	page, err := server.pool.FetchRenderedPage(ctx, browser.PageRequest{
		URL:       params.URL,
		MaxBytes:  params.MaxBytes,
		Headers:   params.Headers,
		UserAgent: params.UserAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch rendered page: %w", err)
	}
	return marshalResult(FetchRenderedResult{URL: page.URL, HTML: page.HTML})
}

func (server *Server) health() Health {
	return Health{
		Status:         "ok",
		Version:        server.version,
		PID:            os.Getpid(),
		StartedAt:      server.startedAt,
		ActiveBrowsers: server.activeBrowsers(),
	}
}

func (server *Server) beginCall() {
	server.activeCalls.Add(1)
	server.activityMu.Lock()
	server.lastActivity = server.clock()
	server.activityMu.Unlock()
}

func (server *Server) endCall() {
	server.activeCalls.Add(-1)
}

func (server *Server) activeBrowsers() int {
	return 0
}

func marshalResult(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode daemon result: %w", err)
	}
	return data, nil
}

func writeReply(writer http.ResponseWriter, status int, reply Reply) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(reply)
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: missing params", errInvalidParams)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %w", errInvalidParams, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: trailing JSON document", errInvalidParams)
		}
		return fmt.Errorf("%w: %w", errInvalidParams, err)
	}
	return nil
}

var (
	errMethodNotFound = errors.New("method not found")
	errInvalidParams  = errors.New("invalid params")
)
