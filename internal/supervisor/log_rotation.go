package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const logBackupSuffix = ".1"

type rotatingWriter struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	maxBytes int64
	size     int64
}

func newRotatingWriter(path string, maxBytes int64) (*rotatingWriter, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultLogMaxBytes
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	//nolint:gosec // The daemon log directory is owner-only.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure log directory: %w", err)
	}

	file, err := openLogPath(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	return &rotatingWriter{
		file:     file,
		path:     path,
		maxBytes: maxBytes,
		size:     info.Size(),
	}, nil
}

func openLogPath(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("log path is not a regular file: %q", path)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("inspect log path: %w", err)
	}

	//nolint:gosec // The log path is absolute and validated by the caller.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure log file: %w", err)
	}
	return file, nil
}

func (writer *rotatingWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	if writer.file == nil {
		return 0, os.ErrClosed
	}
	if writer.size > 0 && writer.size+int64(len(data)) > writer.maxBytes {
		rotationErr := writer.rotateLocked()
		if writer.file == nil {
			return 0, rotationErr
		}
		if rotationErr != nil {
			n, writeErr := writer.file.Write(data)
			writer.size += int64(n)
			return n, errors.Join(rotationErr, writeErr)
		}
	}

	n, err := writer.file.Write(data)
	writer.size += int64(n)
	return n, err
}

func (writer *rotatingWriter) Close() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	writer.size = 0
	return err
}

func (writer *rotatingWriter) rotateLocked() error {
	backupPath := writer.path + logBackupSuffix
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove log backup: %w", err)
	}

	if err := writer.file.Close(); err != nil {
		writer.file = nil
		return fmt.Errorf("close log for rotation: %w", err)
	}
	writer.file = nil

	if err := os.Rename(writer.path, backupPath); err != nil {
		reopenErr := writer.reopenLocked()
		return errors.Join(fmt.Errorf("rename log for rotation: %w", err), reopenErr)
	}
	if err := writer.openLocked(); err != nil {
		restoreErr := os.Rename(backupPath, writer.path)
		reopenErr := writer.reopenLocked()
		return errors.Join(fmt.Errorf("open rotated log: %w", err), restoreErr, reopenErr)
	}
	return nil
}

func (writer *rotatingWriter) reopenLocked() error {
	if writer.file != nil {
		_ = writer.file.Close()
		writer.file = nil
	}
	return writer.openLocked()
}

func (writer *rotatingWriter) openLocked() error {
	file, err := openLogPath(writer.path)
	if err != nil {
		writer.file = nil
		writer.size = 0
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		writer.file = nil
		writer.size = 0
		return fmt.Errorf("stat rotated log: %w", err)
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}
