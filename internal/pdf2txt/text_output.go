package pdf2txt

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func readTextFile(path string, maxBytes int64) (string, error) {
	file, err := os.Open(path) //nolint:gosec // The path is a temp output file created for this extraction.
	if err != nil {
		return "", fmt.Errorf("open text output: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read text output: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("read text output over %d bytes", maxBytes)
	}

	return strings.TrimSpace(string(data)), nil
}
