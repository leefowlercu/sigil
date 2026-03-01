package main

import (
	"os"

	"github.com/leefowlercu/sigil/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
