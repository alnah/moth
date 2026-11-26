//go:build !windows

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

func TestClientHealthStartsDaemonOnDemand(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	var starts atomic.Int32
	var mu sync.Mutex
	var daemons []*inProcessDaemon

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(_ context.Context, spec supervisor.DaemonSpec) error {
			starts.Add(1)
			daemon, err := launchDaemon(spec.RuntimeDir, spec.Version)
			if err != nil {
				return err
			}
			mu.Lock()
			daemons = append(daemons, daemon)
			mu.Unlock()
			return nil
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
		stopDaemons(t, &mu, &daemons)
	})

	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health(first) error = %v, want nil", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("daemon starts = %d, want 1", starts.Load())
	}
	if health.Status != "ok" || health.Version != testVersion || health.PID == 0 {
		t.Fatalf("Health() = %#v, want ok daemon with version and pid", health)
	}

	secondHealth, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health(second) error = %v, want nil", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("daemon starts after second Health = %d, want 1", starts.Load())
	}
	if secondHealth.PID != health.PID {
		t.Fatalf("daemon PID changed from %d to %d", health.PID, secondHealth.PID)
	}
}

func TestClientReusesRunningDaemonWithoutStartingAnother(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)

	var starts atomic.Int32
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(context.Context, supervisor.DaemonSpec) error {
			starts.Add(1)
			return errors.New("must not start a daemon")
		},
	})
	t.Cleanup(func() { _ = client.Close() })

	if _, err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v, want nil", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("daemon starts = %d, want 0 for running daemon", starts.Load())
	}
}

func TestClientHealthReturnsUnavailableWhenStartFails(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	var starts atomic.Int32
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(context.Context, supervisor.DaemonSpec) error {
			starts.Add(1)
			return errors.New("spawn failed")
		},
	})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.Health(context.Background())
	if !errors.Is(err, supervisor.ErrDaemonUnavailable) {
		t.Fatalf("Health() error = %v, want ErrDaemonUnavailable", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("daemon starts = %d, want 1", starts.Load())
	}
}

func TestClientHealthHonorsCanceledContext(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	var starts atomic.Int32
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(context.Context, supervisor.DaemonSpec) error {
			starts.Add(1)
			return nil
		},
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Health(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Health(canceled) error = %v, want context.Canceled", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("daemon starts = %d, want 0 for canceled context", starts.Load())
	}
}

func TestClientHealthRejectsVersionMismatch(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, "daemon-version")

	var starts atomic.Int32
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    "client-version",
		CommandFunc: func(context.Context, supervisor.DaemonSpec) error {
			starts.Add(1)
			return errors.New("must not replace a mismatched daemon")
		},
	})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.Health(context.Background())
	if !errors.Is(err, supervisor.ErrDaemonVersion) {
		t.Fatalf("Health(version mismatch) error = %v, want ErrDaemonVersion", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("daemon starts = %d, want 0 for version mismatch", starts.Load())
	}
}

func TestClientHealthReportsUnauthorizedToken(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)

	runtime := readRuntime(t, runtimeDir)
	runtime.Token = "wrong-token"
	data, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("marshal unauthorized runtime: %v", err)
	}
	if err := os.WriteFile(supervisor.RuntimePath(runtimeDir), data, 0o600); err != nil {
		t.Fatalf("write unauthorized runtime: %v", err)
	}

	var starts atomic.Int32
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(context.Context, supervisor.DaemonSpec) error {
			starts.Add(1)
			return errors.New("must not replace an unauthorized daemon")
		},
	})
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Health(context.Background())
	if !errors.Is(err, supervisor.ErrDaemonAuth) {
		t.Fatalf("Health(unauthorized) error = %v, want ErrDaemonAuth", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("daemon starts = %d, want 0 for unauthorized daemon", starts.Load())
	}
}

func TestClientHealthReplacesCorruptRuntime(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	if err := os.WriteFile(supervisor.RuntimePath(runtimeDir), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt runtime: %v", err)
	}

	var starts atomic.Int32
	var mu sync.Mutex
	var daemons []*inProcessDaemon
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(_ context.Context, spec supervisor.DaemonSpec) error {
			starts.Add(1)
			daemon, err := launchDaemon(spec.RuntimeDir, spec.Version)
			if err != nil {
				return err
			}
			mu.Lock()
			daemons = append(daemons, daemon)
			mu.Unlock()
			return nil
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
		stopDaemons(t, &mu, &daemons)
	})

	if _, err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health(corrupt runtime) error = %v, want nil", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("daemon starts = %d, want 1 after corrupt runtime", starts.Load())
	}
}

func TestClientHealthReplacesStaleRuntime(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	stale := supervisor.Runtime{
		Version:   testVersion,
		PID:       999999,
		Network:   "unix",
		Address:   supervisor.SocketPath(runtimeDir),
		Token:     "stale-token",
		StartedAt: time.Now().Add(-time.Hour),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal stale runtime: %v", err)
	}
	if err := os.WriteFile(supervisor.RuntimePath(runtimeDir), data, 0o600); err != nil {
		t.Fatalf("write stale runtime: %v", err)
	}

	var starts atomic.Int32
	var mu sync.Mutex
	var daemons []*inProcessDaemon
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(_ context.Context, spec supervisor.DaemonSpec) error {
			starts.Add(1)
			daemon, err := launchDaemon(spec.RuntimeDir, spec.Version)
			if err != nil {
				return err
			}
			mu.Lock()
			daemons = append(daemons, daemon)
			mu.Unlock()
			return nil
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
		stopDaemons(t, &mu, &daemons)
	})

	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health(stale runtime) error = %v, want nil", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("daemon starts = %d, want 1 after stale runtime", starts.Load())
	}
	if health.PID == stale.PID {
		t.Fatalf("Health().PID = %d, want new daemon", health.PID)
	}
	fresh := readRuntime(t, runtimeDir)
	if fresh.Token == stale.Token {
		t.Fatal("runtime token was not rotated after stale runtime replacement")
	}
}

func TestClientConcurrentHealthStartsOneDaemon(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	var starts atomic.Int32
	var mu sync.Mutex
	var daemons []*inProcessDaemon

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(_ context.Context, spec supervisor.DaemonSpec) error {
			starts.Add(1)
			daemon, err := launchDaemon(spec.RuntimeDir, spec.Version)
			if err != nil {
				return err
			}
			mu.Lock()
			daemons = append(daemons, daemon)
			mu.Unlock()
			return nil
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
		stopDaemons(t, &mu, &daemons)
	})

	const callers = 10
	var waitGroup sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := client.Health(context.Background())
			errs <- err
		}()
	}
	waitGroup.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Health() error = %v, want nil", err)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("daemon starts = %d, want 1 for concurrent callers", starts.Load())
	}
}

func TestClientShutdownIsIdempotentAndRemovesRuntime(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown(first) error = %v, want nil", err)
	}
	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime file after shutdown err = %v, want removed", err)
	}
	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown(second) error = %v, want nil", err)
	}

	var starts atomic.Int32
	restarting := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(context.Context, supervisor.DaemonSpec) error {
			starts.Add(1)
			return errors.New("spawn failed")
		},
	})
	t.Cleanup(func() { _ = restarting.Close() })

	_, err := restarting.Health(context.Background())
	if !errors.Is(err, supervisor.ErrDaemonUnavailable) {
		t.Fatalf("Health(after shutdown) error = %v, want ErrDaemonUnavailable", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("restart attempts = %d, want 1", starts.Load())
	}
}

func stopDaemons(t *testing.T, mu *sync.Mutex, daemons *[]*inProcessDaemon) {
	t.Helper()

	mu.Lock()
	copied := append([]*inProcessDaemon(nil), (*daemons)...)
	mu.Unlock()
	for _, daemon := range copied {
		if err := daemon.stop(); err != nil {
			t.Errorf("stop daemon: %v", err)
		}
	}
}
