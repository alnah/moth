//go:build windows

package supervisor

func processAlive(pid int) bool {
	return pid > 0
}
