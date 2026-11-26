package supervisor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type methodHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

// Server serves the daemon protocol.
type Server struct {
	version   string
	token     string
	startedAt time.Time
	handlers  map[string]methodHandler

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
	server := &Server{
		version:    options.Version,
		token:      options.Token,
		startedAt:  startedAt,
		handlers:   make(map[string]methodHandler),
		shutdownCh: make(chan struct{}),
	}
	server.handlers[MethodHealth] = server.handleHealth
	server.handlers[MethodShutdown] = server.handleShutdown
	return server
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

func (server *Server) done() <-chan struct{} {
	return server.shutdownCh
}

func (server *Server) requestShutdown() {
	server.shutdownOnce.Do(func() {
		close(server.shutdownCh)
	})
}

func (server *Server) handleCall(writer http.ResponseWriter, request *http.Request) {
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
		code := CodeProtocolError
		if errors.Is(err, errMethodNotFound) {
			code = CodeMethodNotFound
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

func (server *Server) health() Health {
	return Health{
		Status:         "ok",
		Version:        server.version,
		PID:            os.Getpid(),
		StartedAt:      server.startedAt,
		ActiveBrowsers: 0,
	}
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

var errMethodNotFound = errors.New("method not found")
