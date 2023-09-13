package main

import (
	"fmt"
	"os"

	"github.com/canonical/workshop/cmd/cli"
	"github.com/canonical/workshop/internal/logger"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	l, err := logger.New(os.Stderr, 0)
	if err != nil {
		panic(err)
	}

	logger.SetLogger(l)

	rootCmd := (&cli.CmdRoot{}).Command(cwd)

	if err = rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
