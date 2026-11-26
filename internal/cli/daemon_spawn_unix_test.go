//go:build !windows

package cli

import "testing"

func TestDaemonSpawnUsesSetsid(t *testing.T) {
	attributes := daemonSysProcAttr()
	if attributes == nil {
		t.Fatal("daemonSysProcAttr() = nil, want attributes")
	}
	if !attributes.Setsid {
		t.Fatal("daemonSysProcAttr().Setsid = false, want detached session")
	}
}
