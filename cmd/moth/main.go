package main

import (
	"os"

	"github.com/alnah/moth/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
