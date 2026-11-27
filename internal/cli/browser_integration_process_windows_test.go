//go:build browser && windows

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const windowsStillActive uint32 = 259

func init() {
	browserIntegrationPoll = 250 * time.Millisecond
}

func readProcessTable(t *testing.T) map[int]processEntry {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const script = `$ErrorActionPreference = 'Stop'; @(Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,CommandLine) | ConvertTo-Json -Compress`
	//nolint:gosec // The command is fixed and the script does not interpolate input.
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read process table: %v\n%s", err, output)
	}

	payload := strings.TrimSpace(strings.TrimPrefix(string(output), "\ufeff"))
	if payload == "" {
		t.Fatal("read process table: empty output")
	}
	if strings.HasPrefix(payload, "{") {
		payload = "[" + payload + "]"
	}

	var entries []struct {
		ProcessID       int    `json:"ProcessId"`
		ParentProcessID int    `json:"ParentProcessId"`
		CommandLine     string `json:"CommandLine"`
	}
	if err := json.Unmarshal([]byte(payload), &entries); err != nil {
		t.Fatalf("decode process table: %v\n%s", err, payload)
	}

	table := make(map[int]processEntry, len(entries))
	for _, entry := range entries {
		if entry.ProcessID <= 0 {
			continue
		}
		table[entry.ProcessID] = processEntry{
			PID:     entry.ProcessID,
			PPID:    entry.ParentProcessID,
			Command: entry.CommandLine,
		}
	}
	return table
}

func processRunning(entry processEntry) bool {
	return entry.PID > 0 && processAlive(entry.PID)
}

func processAlive(pid int) bool {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActive
}

func assertProcessGroup(*testing.T, processEntry) {}

func hardKillDaemon(t *testing.T, pid int) {
	t.Helper()

	if err := terminateProcess(pid); err != nil {
		t.Fatalf("kill daemon: %v", err)
	}
}

func terminateDaemon(pid int) error {
	return terminateProcess(pid)
}

func terminateChrome(pid int) error {
	return terminateProcess(pid)
}

func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Do not use /T: the leakless guard must observe the daemon socket close and kill Chrome itself.
	//nolint:gosec // The command is fixed and the PID is supplied by the test state.
	output, err := exec.CommandContext(ctx, "taskkill", "/F", "/PID", strconv.Itoa(pid)).CombinedOutput()
	if err == nil || !processAlive(pid) {
		return nil
	}
	return fmt.Errorf("taskkill %d: %w: %s", pid, err, strings.TrimSpace(string(output)))
}
