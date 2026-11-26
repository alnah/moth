package supervisor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	return &Server{
		version:    options.Version,
		token:      options.Token,
		startedAt:  startedAt,
		clock:      clock,
		shutdownCh: make(chan struct{}),
	}
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

	result, err := server.dispatch(call)
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

func (server *Server) dispatch(call Call) (json.RawMessage, error) {
	switch call.Method {
	case MethodHealth:
		return marshalResult(server.health())
	case MethodShutdown:
		server.requestShutdown()
		return marshalResult(server.health())
	default:
		return nil, fmt.Errorf("%w: %s", errMethodNotFound, call.Method)
	}
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

// Run starts a daemon and blocks until its context or a shutdown request.
func Run(ctx context.Context, options RunOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	runtimeDir := strings.TrimSpace(options.RuntimeDir)
	if runtimeDir == "" {
		runtimeDir = defaultRuntimeDir()
	}
	if err := prepareRuntimeDir(runtimeDir); err != nil {
		return err
	}

	runtimeLock, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		return err
	}
	defer func() { _ = runtimeLock.Close() }()

	listener, network, address, err := listen(ctx, runtimeDir)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	token, err := newToken()
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	server := NewServer(ServerOptions{
		Version:   options.Version,
		Token:     token,
		StartedAt: startedAt,
	})
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	runtime := Runtime{
		Version:   options.Version,
		PID:       os.Getpid(),
		Network:   network,
		Address:   address,
		Token:     token,
		StartedAt: startedAt,
	}
	if err := writeRuntime(runtimeDir, runtime); err != nil {
		return err
	}
	defer removeRuntime(runtimeDir)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
	case <-server.shutdownCh:
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve daemon: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultServerTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown daemon: %w", err)
	}
	return nil
}

func defaultRuntimeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".moth", "run")
}
