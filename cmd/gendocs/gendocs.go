package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra/doc"

	"github.com/canonical/workshop/cmd/cli"
)

func filePrepender(filename string) string {
	return ""
}

func linkHandler(name, ref string) string {
	return fmt.Sprintf(":ref:`%s <%s>`", name, ref)
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not get current directory: %v\n", err)
	}

	rootCmd := (&cli.CmdRoot{}).Command(cwd)

	docDir := "docs-auto"
	err = os.MkdirAll(docDir, os.ModePerm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create docs directory: %v\n", err)
	}

	err = doc.GenReSTTreeCustom(rootCmd, filepath.Join(docDir), filePrepender, linkHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate documentation: %v\n", err)
	}
}
