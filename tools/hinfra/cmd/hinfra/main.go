package main

import (
	"os"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
