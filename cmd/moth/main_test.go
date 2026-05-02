package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainStaysThinAndDelegatesToCLIExecute(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "cli.Execute(") {
		t.Fatalf("main.go must delegate top-level execution to cli.Execute; source:\n%s", text)
	}
	if strings.Contains(text, "cli.NewRootCommand(") {
		t.Fatalf("main.go constructs commands directly, want cli.Execute wrapper; source:\n%s", text)
	}
}
