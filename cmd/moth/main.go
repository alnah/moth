package main

import (
	"context"
	"os"

	"github.com/alnah/moth/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
