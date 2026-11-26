package supervisor

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// DefaultLogMaxBytes is the default daemon log rotation threshold.
const DefaultLogMaxBytes int64 = 10 << 20

// LogFile owns the daemon log writer and its JSON logger.
type LogFile struct {
	writer *rotatingWriter
	logger *slog.Logger
}

// OpenLogFile opens or creates an absolute JSON log path.
func OpenLogFile(path string, maxBytes int64) (*LogFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("log path is empty")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("log path must be absolute: %q", path)
	}

	writer, err := newRotatingWriter(path, maxBytes)
	if err != nil {
		return nil, err
	}
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &LogFile{writer: writer, logger: slog.New(handler)}, nil
}

// Logger returns the JSON logger backed by the log file.
func (log *LogFile) Logger() *slog.Logger {
	if log == nil {
		return nil
	}
	return log.logger
}

// Close closes the log file. It is safe to call more than once.
func (log *LogFile) Close() error {
	if log == nil || log.writer == nil {
		return nil
	}
	return log.writer.Close()
}

func openDaemonLogger(options RunOptions) (*LogFile, *slog.Logger, error) {
	if options.Logger != nil {
		return nil, options.Logger, nil
	}
	logPath := strings.TrimSpace(options.LogPath)
	if logPath == "" {
		return nil, nil, nil
	}
	logFile, err := OpenLogFile(logPath, options.LogMaxBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("open daemon log: %w", err)
	}
	logger := logFile.Logger().With(
		"pid", os.Getpid(),
		"version", options.Version,
	)
	return logFile, logger, nil
}
