//go:build !windows

package supervisor_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

const logTestToken = "super-secret-token"

func TestServerLogsCallMetadataWithoutSecrets(t *testing.T) {
	handler, logs := newLoggingHandler(t)
	body := marshalCall(t, supervisor.Call{
		Method: "unknown.method",
		Params: rawParams(t, map[string]string{"secret": "hunter2"}),
	})

	recorder := postLogCall(t, handler, logTestToken, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	record := lastLogRecord(t, logs)
	assertLogField(t, record, "method", "unknown.method")
	assertLogField(t, record, "status", float64(http.StatusOK))
	assertLogField(t, record, "error_code", supervisor.CodeMethodNotFound)
	if record["duration_ms"] == nil {
		t.Fatal("log record has no duration_ms")
	}
	if record["bytes"] == nil {
		t.Fatal("log record has no bytes")
	}
	if record["bytes"] == float64(0) {
		t.Fatal("log record bytes = 0, want response size")
	}
	assertLogDoesNotContain(t, logs, logTestToken)
	assertLogDoesNotContain(t, logs, "hunter2")
}

func TestServerLogsUnauthorizedWithoutToken(t *testing.T) {
	handler, logs := newLoggingHandler(t)
	body := marshalCall(t, supervisor.Call{Method: supervisor.MethodHealth})

	recorder := postLogCall(t, handler, "wrong-token", body)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	record := lastLogRecord(t, logs)
	assertLogField(t, record, "status", float64(http.StatusUnauthorized))
	assertLogField(t, record, "error_code", supervisor.CodeUnauthorized)
	assertLogDoesNotContain(t, logs, logTestToken)
	assertLogDoesNotContain(t, logs, "wrong-token")
}

func TestServerLogsProtocolErrorCode(t *testing.T) {
	handler, logs := newLoggingHandler(t)

	recorder := postLogCall(t, handler, logTestToken, "{")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	record := lastLogRecord(t, logs)
	assertLogField(t, record, "status", float64(http.StatusBadRequest))
	assertLogField(t, record, "error_code", supervisor.CodeProtocolError)
}

func TestServerLogTruncatesMethod(t *testing.T) {
	handler, logs := newLoggingHandler(t)
	method := strings.Repeat("x", 1000)
	body := marshalCall(t, supervisor.Call{Method: method})

	recorder := postLogCall(t, handler, logTestToken, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	record := lastLogRecord(t, logs)
	logged, ok := record["method"].(string)
	if !ok {
		t.Fatalf("log method = %#v, want string", record["method"])
	}
	if len(logged) != 64 {
		t.Fatalf("logged method length = %d, want 64", len(logged))
	}
}

func newLoggingHandler(t *testing.T) (http.Handler, *bytes.Buffer) {
	t.Helper()

	logs := new(bytes.Buffer)
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	server := supervisor.NewServer(supervisor.ServerOptions{
		Version: testVersion,
		Token:   logTestToken,
		Logger:  logger,
	})
	return server.Handler(), logs
}

func postLogCall(t *testing.T, handler http.Handler, token string, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, supervisor.CallPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func lastLogRecord(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n"))
	if len(lines) == 0 {
		t.Fatal("log buffer is empty")
	}
	var record map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &record); err != nil {
		t.Fatalf("decode log record: %v\n%s", err, lines[len(lines)-1])
	}
	return record
}

func assertLogField(t *testing.T, record map[string]any, key string, want any) {
	t.Helper()

	if got := record[key]; got != want {
		t.Fatalf("log %s = %#v, want %#v", key, got, want)
	}
}

func assertLogDoesNotContain(t *testing.T, logs *bytes.Buffer, value string) {
	t.Helper()

	if strings.Contains(logs.String(), value) {
		t.Fatalf("log contains secret %q: %s", value, logs.String())
	}
}
