//go:build !windows

package supervisor_test

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

func TestServerHealthWithValidToken(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)
	runtime := readRuntime(t, runtimeDir)

	client := unixHTTPClient(t, runtime.Address)
	response := postCall(
		t,
		client,
		runtime.Token,
		marshalCall(t, supervisor.Call{Method: supervisor.MethodHealth}),
	)
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		t.Fatalf("health status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	health := decodeHealth(t, decodeReply(t, response))
	if health.Status != "ok" {
		t.Fatalf("health.Status = %q, want ok", health.Status)
	}
	if health.Version != testVersion {
		t.Fatalf("health.Version = %q, want %q", health.Version, testVersion)
	}
	if health.PID != runtime.PID {
		t.Fatalf("health.PID = %d, want runtime PID %d", health.PID, runtime.PID)
	}
}

func TestServerRejectsMissingOrWrongToken(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)
	runtime := readRuntime(t, runtimeDir)
	client := unixHTTPClient(t, runtime.Address)
	body := marshalCall(t, supervisor.Call{Method: supervisor.MethodHealth})

	for _, token := range []string{"", "wrong-token"} {
		response := postCall(t, client, token, body)
		if response.StatusCode != http.StatusUnauthorized {
			_ = response.Body.Close()
			t.Fatalf("token %q status = %d, want %d", token, response.StatusCode, http.StatusUnauthorized)
		}
		_ = response.Body.Close()
	}
}

func TestServerRejectsMalformedRequest(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)
	runtime := readRuntime(t, runtimeDir)
	client := unixHTTPClient(t, runtime.Address)

	response := postCall(t, client, runtime.Token, "{")
	if response.StatusCode != http.StatusBadRequest {
		_ = response.Body.Close()
		t.Fatalf("malformed request status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
	reply := decodeReply(t, response)
	if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
		t.Fatalf("malformed request error = %#v, want code %q", reply.Error, supervisor.CodeProtocolError)
	}
}

func TestServerRejectsUnknownMethod(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)
	runtime := readRuntime(t, runtimeDir)
	client := unixHTTPClient(t, runtime.Address)

	response := postCall(
		t,
		client,
		runtime.Token,
		marshalCall(t, supervisor.Call{Method: "unknown.method"}),
	)
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		t.Fatalf("unknown method status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	reply := decodeReply(t, response)
	if reply.Error == nil || reply.Error.Code != supervisor.CodeMethodNotFound {
		t.Fatalf("unknown method error = %#v, want code %q", reply.Error, supervisor.CodeMethodNotFound)
	}
}

func TestServerRejectsOversizedRequest(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)
	runtime := readRuntime(t, runtimeDir)
	client := unixHTTPClient(t, runtime.Address)

	response := postCall(t, client, runtime.Token, strings.Repeat(" ", (1<<20)+1))
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		_ = response.Body.Close()
		t.Fatalf(
			"oversized request status = %d, want %d",
			response.StatusCode,
			http.StatusRequestEntityTooLarge,
		)
	}
	_ = response.Body.Close()
}

func TestServerHealthRequiresPost(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)
	runtime := readRuntime(t, runtimeDir)
	client := unixHTTPClient(t, runtime.Address)

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://moth"+supervisor.CallPath,
		nil,
	)
	if err != nil {
		t.Fatalf("create GET request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+runtime.Token)

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET health status = %d, want %d", response.StatusCode, http.StatusMethodNotAllowed)
	}
}
