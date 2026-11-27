//go:build browser

package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

type processEntry struct {
	PID     int
	PPID    int
	PGID    int
	Stat    string
	Command string
}

type processTree struct {
	Daemon processEntry
	Guard  processEntry
	Chrome processEntry
}

func processEntryFor(t *testing.T, table map[int]processEntry, pid int) processEntry {
	t.Helper()

	entry, ok := table[pid]
	if !ok {
		t.Fatalf("process %d not found", pid)
	}
	return entry
}

func waitProcessGone(t *testing.T, pid int) {
	t.Helper()

	waitFor(t, browserIntegrationStop, func() error {
		entry, ok := readProcessTable(t)[pid]
		if !ok || !processRunning(entry) {
			return nil
		}
		return fmt.Errorf("pid %d still alive: %s", pid, entry.Command)
	})
}

func (env *browserIntegrationEnv) assertOwnedTree(
	t *testing.T,
	runtimeState supervisor.Runtime,
	state browserStateFile,
) processTree {
	t.Helper()

	if state.ChromePID <= 0 {
		t.Fatalf("chrome pid = %d, want positive", state.ChromePID)
	}
	if !pathWithin(env.Work, state.DataDir) {
		t.Fatalf("chrome data dir = %q, want under %q", state.DataDir, env.Work)
	}

	table := readProcessTable(t)
	daemon := processEntryFor(t, table, runtimeState.PID)
	chrome := processEntryFor(t, table, state.ChromePID)
	guard := processEntryFor(t, table, chrome.PPID)

	assertProcessGroup(t, chrome)
	if chrome.PPID != guard.PID {
		t.Fatalf("chrome ppid = %d, want guard %d", chrome.PPID, guard.PID)
	}
	if guard.PPID != daemon.PID {
		t.Fatalf("guard ppid = %d, want daemon %d", guard.PPID, daemon.PID)
	}
	if !strings.Contains(strings.ToLower(guard.Command), "leakless") {
		t.Fatalf("guard command = %q, want leakless", guard.Command)
	}
	if !commandContainsPath(chrome.Command, state.DataDir) {
		t.Fatalf("chrome command = %q, want data dir %q", chrome.Command, state.DataDir)
	}
	return processTree{Daemon: daemon, Guard: guard, Chrome: chrome}
}

func (env *browserIntegrationEnv) waitPoolTree(t *testing.T, daemonPID int) processTree {
	t.Helper()

	var tree processTree
	waitFor(t, browserIntegrationStop, func() error {
		table := readProcessTable(t)
		daemon := processEntryFor(t, table, daemonPID)
		for _, guard := range table {
			if guard.PPID != daemonPID || !strings.Contains(strings.ToLower(guard.Command), "leakless") {
				continue
			}
			for _, chrome := range table {
				if chrome.PPID != guard.PID {
					continue
				}
				if !commandContainsPath(chrome.Command, env.poolProfile()) {
					continue
				}
				tree = processTree{Daemon: daemon, Guard: guard, Chrome: chrome}
				return nil
			}
		}
		return fmt.Errorf("pool Chrome not found under daemon %d: %s", daemonPID, processSummary(table))
	})
	return tree
}

func (env *browserIntegrationEnv) assertNoTestChrome(t *testing.T) {
	t.Helper()

	persistentProfile := filepath.Join(env.Work, ".moth", "browser", "chrome-data")
	poolProfile := env.poolProfile()
	for _, entry := range readProcessTable(t) {
		if !processRunning(entry) {
			continue
		}
		if commandContainsPath(entry.Command, persistentProfile) || commandContainsPath(entry.Command, poolProfile) {
			t.Fatalf("test Chrome still alive: pid=%d command=%s", entry.PID, entry.Command)
		}
	}
}

func commandContainsPath(command string, path string) bool {
	command = strings.ToLower(filepath.ToSlash(command))
	path = strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(command, path)
}

func processSummary(table map[int]processEntry) string {
	var summary strings.Builder
	for _, entry := range table {
		command := strings.ToLower(entry.Command)
		if !strings.Contains(command, "chrome") && !strings.Contains(command, "leakless") {
			continue
		}
		if summary.Len() > 0 {
			summary.WriteString("; ")
		}
		fmt.Fprintf(&summary, "pid=%d ppid=%d command=%s", entry.PID, entry.PPID, entry.Command)
	}
	return summary.String()
}
