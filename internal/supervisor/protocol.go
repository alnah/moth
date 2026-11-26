package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const (
	// CallPath is the single HTTP endpoint exposed by the daemon.
	CallPath = "/v1/call"

	// MethodHealth reports daemon liveness and version.
	MethodHealth = "daemon.health"
	// MethodShutdown asks the daemon to stop.
	MethodShutdown = "daemon.shutdown"

	// CodeProtocolError reports malformed or unsupported requests.
	CodeProtocolError = "daemon_protocol_error"
	// CodeMethodNotFound reports an unknown daemon method.
	CodeMethodNotFound = "method_not_found"
	// CodeUnauthorized reports a missing or invalid daemon token.
	CodeUnauthorized = "daemon_unauthorized"

	daemonProbeAttempts = 2
	maxRequestBody      = 1 << 20

	defaultHealthTimeout = 2 * time.Second
	defaultShutdownWait  = 5 * time.Second
	defaultServerTimeout = 5 * time.Second
)

var (
	// ErrDaemonUnavailable reports that no daemon could be reached or started.
	ErrDaemonUnavailable = errors.New("daemon unavailable")
	// ErrDaemonVersion reports a client/daemon version mismatch.
	ErrDaemonVersion = errors.New("daemon version mismatch")
	// ErrDaemonAuth reports a missing or invalid daemon token.
	ErrDaemonAuth = errors.New("daemon unauthorized")
	// ErrDaemonAlreadyRunning reports an existing daemon for the runtime directory.
	ErrDaemonAlreadyRunning = errors.New("daemon already running")
	// ErrDaemonProtocol reports a malformed daemon response.
	ErrDaemonProtocol = errors.New("daemon protocol error")
)

// Runtime describes the durable endpoint and credentials of a running daemon.
type Runtime struct {
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	Network   string    `json:"network"`
	Address   string    `json:"address"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
}

// Health describes a live daemon.
type Health struct {
	Status         string    `json:"status"`
	Version        string    `json:"version"`
	PID            int       `json:"pid"`
	StartedAt      time.Time `json:"started_at"`
	ActiveBrowsers int       `json:"active_browsers"`
}

// Call is one daemon request.
type Call struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Reply is one daemon response.
type Reply struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *CallError      `json:"error,omitempty"`
}

// CallError is a structured daemon error.
type CallError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Endpoint identifies a daemon socket and bearer token.
type Endpoint struct {
	Network string
	Address string
	Token   string
}

// DaemonSpec describes how to start a daemon.
type DaemonSpec struct {
	RuntimeDir string
	Version    string
	BrowserBin string
}

// ClientOptions configures a daemon client.
type ClientOptions struct {
	RuntimeDir  string
	Version     string
	BrowserBin  string
	Executable  string
	HTTPClient  *http.Client
	CommandFunc func(context.Context, DaemonSpec) error
}

// RunOptions configures a daemon process.
type RunOptions struct {
	RuntimeDir  string
	Version     string
	BrowserBin  string
	IdleTimeout time.Duration
}

// ServerOptions configures a daemon HTTP server.
type ServerOptions struct {
	Version   string
	Token     string
	StartedAt time.Time
	Clock     func() time.Time
}
