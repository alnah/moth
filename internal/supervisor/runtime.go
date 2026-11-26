package supervisor

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	runtimeFileName = "daemon.json"
	socketFileName  = "moth.sock"
	lockFileName    = "daemon.lock"
)

// RuntimePath returns the daemon runtime file inside runtimeDir.
func RuntimePath(runtimeDir string) string {
	return filepath.Join(runtimeDir, runtimeFileName)
}

// SocketPath returns the Unix socket path inside runtimeDir.
func SocketPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, socketFileName)
}

// ReadRuntime reads and validates the daemon runtime file.
func ReadRuntime(runtimeDir string) (Runtime, error) {
	data, err := os.ReadFile(RuntimePath(runtimeDir))
	if err != nil {
		return Runtime{}, err
	}

	var runtime Runtime
	if err := json.Unmarshal(data, &runtime); err != nil {
		return Runtime{}, fmt.Errorf("decode runtime file: %w", err)
	}
	if strings.TrimSpace(runtime.Address) == "" || strings.TrimSpace(runtime.Token) == "" {
		return Runtime{}, errors.New("runtime file missing address or token")
	}
	return runtime, nil
}

func writeRuntime(runtimeDir string, runtime Runtime) error {
	data, err := json.MarshalIndent(runtime, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime file: %w", err)
	}

	path := RuntimePath(runtimeDir)
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write runtime file: %w", err)
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("secure runtime file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace runtime file: %w", err)
	}
	return nil
}

func prepareRuntimeDir(runtimeDir string) error {
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	//nolint:gosec // The runtime directory must be traversable by the owner.
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return fmt.Errorf("secure runtime directory: %w", err)
	}
	return nil
}

func removeRuntime(runtimeDir string) {
	_ = os.Remove(RuntimePath(runtimeDir))
}

func newToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate daemon token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
