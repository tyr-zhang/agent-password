package main

import (
	"os"

	"github.com/tyr-zhang/agent-password/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
