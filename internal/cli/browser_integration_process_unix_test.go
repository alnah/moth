//go:build browser && !windows

package cli

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

func processRunning(entry processEntry) bool {
	return entry.PID > 0 && !strings.HasPrefix(entry.Stat, "Z")
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func assertProcessGroup(t *testing.T, entry processEntry) {
	t.Helper()

	if entry.PGID != entry.PID {
		t.Fatalf("chrome pgid = %d, want pid %d", entry.PGID, entry.PID)
	}
}

func hardKillDaemon(t *testing.T, pid int) {
	t.Helper()

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill daemon: %v", err)
	}
}

func terminateDaemon(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func terminateChrome(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}
