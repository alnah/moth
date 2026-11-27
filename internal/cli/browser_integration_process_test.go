//go:build browser && !windows

package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

func readProcessTable(t *testing.T) map[int]processEntry {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "ps", "-axww", "-o", "pid=,ppid=,pgid=,stat=,command=").Output()
	if err != nil {
		t.Fatalf("read process table: %v", err)
	}

	table := make(map[int]processEntry)
	for line := range strings.SplitSeq(string(output), "\n") {
		entry, ok := parseProcessLine(line)
		if ok {
			table[entry.PID] = entry
		}
	}
	return table
}

func parseProcessLine(line string) (processEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return processEntry{}, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return processEntry{}, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return processEntry{}, false
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return processEntry{}, false
	}
	return processEntry{
		PID:     pid,
		PPID:    ppid,
		PGID:    pgid,
		Stat:    fields[3],
		Command: strings.Join(fields[4:], " "),
	}, true
}

func processEntryFor(t *testing.T, table map[int]processEntry, pid int) processEntry {
	t.Helper()

	entry, ok := table[pid]
	if !ok {
		t.Fatalf("process %d not found", pid)
	}
	return entry
}

func processRunning(entry processEntry) bool {
	return entry.PID > 0 && !strings.HasPrefix(entry.Stat, "Z")
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

	if chrome.PGID != chrome.PID {
		t.Fatalf("chrome pgid = %d, want pid %d", chrome.PGID, chrome.PID)
	}
	if chrome.PPID != guard.PID {
		t.Fatalf("chrome ppid = %d, want guard %d", chrome.PPID, guard.PID)
	}
	if guard.PPID != daemon.PID {
		t.Fatalf("guard ppid = %d, want daemon %d", guard.PPID, daemon.PID)
	}
	if !strings.Contains(guard.Command, "leakless") {
		t.Fatalf("guard command = %q, want leakless", guard.Command)
	}
	if !strings.Contains(chrome.Command, state.DataDir) {
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
			if guard.PPID != daemonPID || !strings.Contains(guard.Command, "leakless") {
				continue
			}
			for _, chrome := range table {
				if chrome.PPID != guard.PID {
					continue
				}
				if !strings.Contains(chrome.Command, env.Tmp) ||
					!strings.Contains(chrome.Command, "/rod/user-data") {
					continue
				}
				tree = processTree{Daemon: daemon, Guard: guard, Chrome: chrome}
				return nil
			}
		}
		return fmt.Errorf("pool Chrome not found under daemon %d", daemonPID)
	})
	return tree
}

func (env *browserIntegrationEnv) assertNoTestChrome(t *testing.T) {
	t.Helper()

	persistentProfile := filepath.Join(env.Work, ".moth", "browser", "chrome-data")
	poolProfile := filepath.Join(env.Tmp, "rod", "user-data")
	for _, entry := range readProcessTable(t) {
		if !processRunning(entry) {
			continue
		}
		if strings.Contains(entry.Command, persistentProfile) || strings.Contains(entry.Command, poolProfile) {
			t.Fatalf("test Chrome still alive: pid=%d command=%s", entry.PID, entry.Command)
		}
	}
}
