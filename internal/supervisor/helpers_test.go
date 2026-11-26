//go:build !windows

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

const (
	testVersion      = "test-version"
	testStartTimeout = 2 * time.Second
	testStopTimeout  = 2 * time.Second
)

type inProcessDaemon struct {
	cancel   context.CancelFunc
	done     chan error
	stopOnce sync.Once
	stopErr  error
	stopped  chan struct{}
}

func launchDaemon(runtimeDir string, version string) (*inProcessDaemon, error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	daemon := &inProcessDaemon{
		cancel:  cancel,
		done:    done,
		stopped: make(chan struct{}),
	}
	go func() {
		done <- supervisor.Run(ctx, supervisor.RunOptions{
			RuntimeDir: runtimeDir,
			Version:    version,
		})
	}()

	if err := waitForRuntimeFile(runtimeDir, testStartTimeout); err != nil {
		_ = daemon.stop()
		return nil, err
	}
	return daemon, nil
}

func startDaemonAt(t *testing.T, runtimeDir string, version string) *inProcessDaemon {
	t.Helper()

	daemon, err := launchDaemon(runtimeDir, version)
	if err != nil {
		t.Fatalf("start daemon at %q: %v", runtimeDir, err)
	}
	t.Cleanup(func() {
		if err := daemon.stop(); err != nil {
			t.Errorf("stop daemon at %q: %v", runtimeDir, err)
		}
	})
	return daemon
}

func (daemon *inProcessDaemon) stop() error {
	if daemon == nil {
		return nil
	}
	daemon.stopOnce.Do(func() {
		daemon.cancel()
		select {
		case err := <-daemon.done:
			if errors.Is(err, context.Canceled) {
				err = nil
			}
			daemon.stopErr = err
		case <-time.After(testStopTimeout):
			daemon.stopErr = errors.New("daemon did not stop")
		}
		close(daemon.stopped)
	})
	<-daemon.stopped
	return daemon.stopErr
}

func waitForRuntimeFile(runtimeDir string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runtime, err := supervisor.ReadRuntime(runtimeDir)
		if err == nil && runtime.Address != "" && runtime.Token != "" {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("runtime file in %q not ready after %s", runtimeDir, timeout)
}

func readRuntime(t *testing.T, runtimeDir string) supervisor.Runtime {
	t.Helper()

	runtime, err := supervisor.ReadRuntime(runtimeDir)
	if err != nil {
		t.Fatalf("read runtime: %v", err)
	}
	return runtime
}

func unixHTTPClient(t *testing.T, socketPath string) *http.Client {
	t.Helper()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: true,
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 2 * time.Second}
}

func postCall(
	t *testing.T,
	client *http.Client,
	token string,
	body string,
) *http.Response {
	t.Helper()

	request, err := http.NewRequest(
		http.MethodPost,
		"http://moth"+supervisor.CallPath,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create call request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("post call: %v", err)
	}
	return response
}

func decodeReply(t *testing.T, response *http.Response) supervisor.Reply {
	t.Helper()
	defer func() { _ = response.Body.Close() }()

	var reply supervisor.Reply
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	return reply
}

func decodeHealth(t *testing.T, reply supervisor.Reply) supervisor.Health {
	t.Helper()

	if reply.Error != nil {
		t.Fatalf("reply error = %#v, want nil", reply.Error)
	}
	var health supervisor.Health
	if err := json.Unmarshal(reply.Result, &health); err != nil {
		t.Fatalf("decode health result: %v", err)
	}
	return health
}

func marshalCall(t *testing.T, call supervisor.Call) string {
	t.Helper()

	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal call: %v", err)
	}
	return string(data)
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func writeStaleSocket(t *testing.T, socketPath string) {
	t.Helper()

	address, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		t.Fatalf("resolve stale socket: %v", err)
	}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		t.Fatalf("listen stale socket: %v", err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatalf("close stale socket: %v", err)
	}
}
