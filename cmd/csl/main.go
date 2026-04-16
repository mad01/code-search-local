package main

import (
	"os"

	"github.com/mad01/code-search-local/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
