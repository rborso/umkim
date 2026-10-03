package main

import (
	"os"

	"github.com/rborso/umkim/cmd/umkim/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
