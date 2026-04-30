package pdf2txt

import (
	"fmt"
	"os"
)

type ownedWorkDir struct {
	path string
}

func (extractor *Extractor) createWorkDir() (ownedWorkDir, error) {
	root := extractor.tempDir
	if root == "" {
		root = os.TempDir()
	}
	path, err := os.MkdirTemp(root, "moth-pdf2txt-*")
	if err != nil {
		return ownedWorkDir{}, fmt.Errorf("create PDF extraction temp dir: %w", err)
	}

	return ownedWorkDir{path: path}, nil
}

func (workDir ownedWorkDir) close() {
	if workDir.path == "" {
		return
	}
	_ = os.RemoveAll(workDir.path)
}
