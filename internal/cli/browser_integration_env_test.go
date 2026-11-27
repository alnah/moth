//go:build browser

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ysmood/leakless"

	"github.com/alnah/moth/internal/supervisor"
	"github.com/alnah/moth/internal/tools"
)

const (
	browserRequiredEnv       = "MOTH_BROWSER_REQUIRED"
	browserIntegrationWait   = 30 * time.Second
	browserIntegrationStop   = 10 * time.Second
	browserIntegrationOutput = 45 * time.Second
)

var browserIntegrationPoll = 50 * time.Millisecond

type browserIntegrationEnv struct {
	Home       string
	Work       string
	Tmp        string
	Binary     string
	BrowserBin string
	RuntimeDir string

	env []string
}

type commandResult struct {
	Stdout string
	Stderr string
	Err    error
}

type browserCLIProcess struct {
	Command *exec.Cmd
	Stdout  *bytes.Buffer
	Stderr  *bytes.Buffer

	done    chan struct{}
	waitErr error
}

type browserStateFile struct {
	DebugURL     string `json:"debug_url"`
	ChromePID    int    `json:"chrome_pid"`
	Owned        bool   `json:"owned"`
	DataDir      string `json:"data_dir"`
	ActivePageID string `json:"active_page_id"`
}

type browserStatusEnvelope struct {
	Type   string                         `json:"type"`
	Status supervisor.BrowserStatusResult `json:"status"`
}

type browserPageEnvelope struct {
	Type string                       `json:"type"`
	Page supervisor.BrowserPageResult `json:"page"`
}

type browserPagesEnvelope struct {
	Type  string                         `json:"type"`
	Pages []supervisor.BrowserPageResult `json:"pages"`
}

type daemonStatusEnvelope struct {
	Type   string             `json:"type"`
	Status string             `json:"status"`
	Health *supervisor.Health `json:"health"`
}

func newBrowserIntegrationEnv(t *testing.T) *browserIntegrationEnv {
	t.Helper()

	if !leakless.Support() {
		t.Skipf("real browser integration requires leakless support on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	home := mustMkdirTemp(t)
	if runtime.GOOS == "windows" {
		// Chrome resolves its profile from USERPROFILE on Windows.
		for _, dir := range []string{"Local", "Roaming"} {
			if err := os.MkdirAll(filepath.Join(home, "AppData", dir), 0o750); err != nil {
				t.Fatalf("create test profile AppData: %v", err)
			}
		}
	}
	work := mustMkdirTemp(t)
	tmp := mustMkdirTemp(t)
	t.Cleanup(func() {
		_ = os.RemoveAll(home)
		_ = os.RemoveAll(work)
		_ = os.RemoveAll(tmp)
	})

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	env := &browserIntegrationEnv{
		Home:       home,
		Work:       work,
		Tmp:        tmp,
		Binary:     binary,
		RuntimeDir: filepath.Join(home, ".moth", "run"),
	}
	env.env = env.environment()
	t.Cleanup(func() { env.cleanup(t) })
	return env
}

func mustMkdirTemp(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "moth-integration-")
	if err != nil {
		t.Fatalf("create temporary directory: %v", err)
	}
	return dir
}

func (env *browserIntegrationEnv) environment() []string {
	environment := daemonEnvironment(os.Environ())
	environment = setEnv(environment, "HOME", env.Home)
	environment = setEnv(environment, "USERPROFILE", env.Home)
	environment = setEnv(environment, "TMPDIR", env.Tmp)
	environment = setEnv(environment, "TMP", env.Tmp)
	environment = setEnv(environment, "TEMP", env.Tmp)
	return environment
}

func setEnv(environment []string, name string, value string) []string {
	prefix := name + "="
	filtered := environment[:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}

func (env *browserIntegrationEnv) requireBrowser(t *testing.T) {
	t.Helper()

	resolved, err := tools.ResolveBrowser(
		t.Context(),
		tools.BrowserDoctorOptions{SearchCommonInstallPaths: true},
		tools.Platform{OS: runtime.GOOS},
	)
	if err != nil {
		if os.Getenv(browserRequiredEnv) == "1" {
			t.Fatalf("browser unavailable while %s=1: %v", browserRequiredEnv, err)
		}
		t.Skipf("browser unavailable: %v", err)
	}
	env.BrowserBin = resolved.Path
	env.env = setEnv(env.env, "ROD_BROWSER_BIN", resolved.Path)
}

func (env *browserIntegrationEnv) command(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()

	//nolint:gosec,noctx // The test executable path comes from os.Executable.
	cmd := exec.Command(env.Binary, args...)
	cmd.Dir = env.Work
	cmd.Env = append([]string(nil), env.env...)
	return cmd
}

func (env *browserIntegrationEnv) run(t *testing.T, args ...string) commandResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), browserIntegrationOutput)
	defer cancel()

	//nolint:gosec // The test executable path comes from os.Executable.
	cmd := exec.CommandContext(ctx, env.Binary, args...)
	cmd.Dir = env.Work
	cmd.Env = append([]string(nil), env.env...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return commandResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

func (env *browserIntegrationEnv) start(t *testing.T, args ...string) *browserCLIProcess {
	t.Helper()

	cmd := env.command(t, args...)
	process := &browserCLIProcess{
		Command: cmd,
		Stdout:  new(bytes.Buffer),
		Stderr:  new(bytes.Buffer),
		done:    make(chan struct{}),
	}
	cmd.Stdout = process.Stdout
	cmd.Stderr = process.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start CLI: %v", err)
	}
	go func() {
		process.waitErr = cmd.Wait()
		close(process.done)
	}()
	t.Cleanup(func() { process.cleanup(t) })
	return process
}

func (process *browserCLIProcess) wait(t *testing.T, timeout time.Duration) error {
	t.Helper()

	select {
	case <-process.done:
		return process.waitErr
	case <-time.After(timeout):
		t.Fatalf("CLI did not exit within %s\nstdout: %s\nstderr: %s", timeout, process.Stdout, process.Stderr)
		return nil
	}
}

func (process *browserCLIProcess) kill(t *testing.T) {
	t.Helper()

	if process == nil || process.Command == nil || process.Command.Process == nil {
		return
	}
	if err := process.Command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("kill CLI: %v", err)
	}
}

func (process *browserCLIProcess) cleanup(t *testing.T) {
	t.Helper()

	if process == nil || process.Command == nil || process.Command.Process == nil {
		return
	}
	select {
	case <-process.done:
		return
	default:
	}
	_ = process.Command.Process.Kill()
	select {
	case <-process.done:
	case <-time.After(time.Second):
	}
}

func (env *browserIntegrationEnv) stopDaemon(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//nolint:gosec // The test executable path comes from os.Executable.
	cmd := exec.CommandContext(ctx, env.Binary, "daemon", "stop")
	cmd.Dir = env.Work
	cmd.Env = append([]string(nil), env.env...)
	_ = cmd.Run()
}

func (env *browserIntegrationEnv) cleanup(t *testing.T) {
	t.Helper()

	env.cleanupProcesses(t)
}

func (env *browserIntegrationEnv) cleanupProcesses(t *testing.T) {
	t.Helper()

	runtimeState, runtimeErr := supervisor.ReadRuntime(env.RuntimeDir)
	if runtimeErr == nil {
		env.stopDaemon(t)
		if processAlive(runtimeState.PID) {
			_ = terminateDaemon(runtimeState.PID)
		}
	}

	state, err := readBrowserState(env.localStatePath())
	if err != nil || !state.Owned || state.ChromePID <= 0 {
		return
	}
	if !pathWithin(env.Work, state.DataDir) {
		return
	}
	table := readProcessTable(t)
	chrome, ok := table[state.ChromePID]
	if !ok || !strings.Contains(chrome.Command, state.DataDir) {
		return
	}
	_ = terminateChrome(state.ChromePID)
}

func (env *browserIntegrationEnv) poolProfile() string {
	return filepath.Join(env.Tmp, "rod", "user-data")
}

func (env *browserIntegrationEnv) localStatePath() string {
	return filepath.Join(env.Work, ".moth", "browser", "state.json")
}

func readBrowserState(path string) (browserStateFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Test path is isolated under Work.
	if err != nil {
		return browserStateFile{}, err
	}
	var state browserStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return browserStateFile{}, fmt.Errorf("decode browser state: %w", err)
	}
	return state, nil
}

func pathWithin(root string, path string) bool {
	root = canonicalPath(root)
	path = canonicalPath(path)
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func canonicalPath(path string) string {
	cleaned := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return resolved
}

func waitFor(t *testing.T, timeout time.Duration, check func() error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = check()
		if lastErr == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met after %s: %v", timeout, lastErr)
		}
		time.Sleep(browserIntegrationPoll)
	}
}

func (env *browserIntegrationEnv) waitRuntime(t *testing.T) supervisor.Runtime {
	t.Helper()

	var runtimeState supervisor.Runtime
	waitFor(t, browserIntegrationWait, func() error {
		var err error
		runtimeState, err = supervisor.ReadRuntime(env.RuntimeDir)
		return err
	})
	return runtimeState
}

func (env *browserIntegrationEnv) waitState(t *testing.T) browserStateFile {
	t.Helper()

	var state browserStateFile
	waitFor(t, browserIntegrationWait, func() error {
		var err error
		state, err = readBrowserState(env.localStatePath())
		return err
	})
	return state
}

func (env *browserIntegrationEnv) waitStateGone(t *testing.T) {
	t.Helper()

	waitFor(t, browserIntegrationStop, func() error {
		_, err := os.Stat(env.localStatePath())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

func (env *browserIntegrationEnv) waitRuntimeGone(t *testing.T) {
	t.Helper()

	waitFor(t, browserIntegrationStop, func() error {
		_, err := os.Stat(supervisor.RuntimePath(env.RuntimeDir))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

func decodeJSON[T any](t *testing.T, output string) T {
	t.Helper()

	var value T
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatalf("decode JSON: %v\noutput: %s", err, output)
	}
	return value
}

func assertNoToken(t *testing.T, log string, token string) {
	t.Helper()

	if token != "" && strings.Contains(log, token) {
		t.Fatal("daemon log contains the runtime token")
	}
}
