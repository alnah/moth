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

type methodHandler func(
	context.Context,
	requestMetadata,
	json.RawMessage,
) (json.RawMessage, error)

// Server serves the daemon protocol.
type Server struct {
	version           string
	token             string
	startedAt         time.Time
	clock             func() time.Time
	lifecycle         browser.LifecycleOptions
	logger            *slog.Logger
	pool              *browser.Pool
	browserBin        string
	newBrowserService BrowserServiceFactory
	handlers          map[string]methodHandler

	browserMu sync.Mutex
	browsers  map[browserServiceKey]*browser.PersistentService

	activityMu   sync.Mutex
	lastActivity time.Time
	activeCalls  atomic.Int64

	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error

	callCtx     context.Context
	cancelCalls context.CancelFunc

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
	factory := options.PersistentServiceFactory
	if factory == nil {
		factory = func(options browser.PersistentServiceOptions) *browser.PersistentService {
			if options.TerminateProcess == nil {
				options.TerminateProcess = browser.DefaultTerminateProcess
			}
			return browser.NewPersistentService(options)
		}
	}
	callCtx, cancelCalls := context.WithCancel(context.Background())
	server := &Server{
		version:           options.Version,
		token:             options.Token,
		startedAt:         startedAt,
		clock:             clock,
		lifecycle:         options.Lifecycle,
		logger:            logger,
		pool:              options.Pool,
		browserBin:        options.BrowserBin,
		newBrowserService: factory,
		browsers:          make(map[browserServiceKey]*browser.PersistentService),
		handlers:          make(map[string]methodHandler),
		lastActivity:      startedAt,
		callCtx:           callCtx,
		cancelCalls:       cancelCalls,
		shutdownCh:        make(chan struct{}),
	}
	server.handlers[MethodHealth] = server.handleHealth
	server.handlers[MethodShutdown] = server.handleShutdown
	server.handlers[MethodFetchRendered] = server.handleFetchRendered
	server.registerBrowserHandlers()
	return server
}

// Handler returns the daemon HTTP handler.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(CallPath, server.handleCall)
	return mux
}

func (server *Server) handleCall(writer http.ResponseWriter, request *http.Request) {
	recorder := &statusRecorder{ResponseWriter: writer}
	writer = recorder
	started := server.clock()
	ctx, cancel := server.callContext(request.Context())
	defer cancel()
	var call Call
	errorCode := ""
	defer func() {
		server.logCall(call, recorder, errorCode, started)
	}()

	server.beginCall()
	defer server.endCall()
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !server.authorized(request) {
		errorCode = CodeUnauthorized
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

	if err := json.NewDecoder(body).Decode(&call); err != nil {
		errorCode = CodeProtocolError
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
		errorCode = CodeProtocolError
		writeReply(writer, http.StatusBadRequest, Reply{
			Error: &CallError{
				Code:    CodeProtocolError,
				Message: "missing daemon method",
			},
		})
		return
	}

	result, err := server.dispatch(ctx, requestMetadataFrom(request), call)
	if err != nil {
		code := CodeMethodFailed
		switch {
		case errors.Is(err, errMethodNotFound):
			code = CodeMethodNotFound
		case errors.Is(err, errInvalidParams):
			code = CodeProtocolError
		case errors.Is(err, browser.ErrBrowserStateCorrupt):
			code = CodeBrowserStateCorrupt
		case errors.Is(err, browser.ErrBrowserStateUnavailable):
			code = CodeBrowserStateUnavailable
		}
		errorCode = code
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

func (server *Server) dispatch(
	ctx context.Context,
	metadata requestMetadata,
	call Call,
) (json.RawMessage, error) {
	handler, ok := server.handlers[call.Method]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errMethodNotFound, call.Method)
	}
	return handler(ctx, metadata, call.Params)
}

func (server *Server) handleHealth(
	context.Context,
	requestMetadata,
	json.RawMessage,
) (json.RawMessage, error) {
	return marshalResult(server.health())
}

func (server *Server) handleShutdown(
	context.Context,
	requestMetadata,
	json.RawMessage,
) (json.RawMessage, error) {
	server.requestShutdown()
	return marshalResult(server.health())
}

func (server *Server) handleFetchRendered(
	ctx context.Context,
	_ requestMetadata,
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
