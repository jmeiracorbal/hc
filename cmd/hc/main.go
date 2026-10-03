package main

import (
	"fmt"
	"os"

	"github.com/jmeiracorbal/hybrid-coco/internal/cli"
)

// set via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cli.SetVersion(version)
	root := cli.NewRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
