package main

import (
	"os"
	"testing"
)

func TestMainReturnsForRootHelp(t *testing.T) {
	originalArgs := os.Args
	originalStdout := os.Stdout
	t.Cleanup(func() {
		os.Args = originalArgs
		os.Stdout = originalStdout
	})

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open dev null: %v", err)
	}
	t.Cleanup(func() {
		if err := devNull.Close(); err != nil {
			t.Fatalf("close dev null: %v", err)
		}
	})

	os.Args = []string{"moth"}
	os.Stdout = devNull

	main()
}
