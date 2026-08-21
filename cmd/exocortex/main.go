package main

import (
	"os"

	"github.com/misty-step/exocortex/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
