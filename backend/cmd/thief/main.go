package main

import (
	"os"

	"github.com/sfuruya0612/thief/backend/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
