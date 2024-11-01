package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/canonical/workshop/cmd/cli"
)

func filePrepender(filename string) string {
	return ""
}

func linkHandler(name, ref string) string {
	return fmt.Sprintf(":ref:`%s <%s>`", name, ref)
}

func main() {
	docDir := "docs-gendocs"
	if len(os.Args) > 1 {
		docDir = os.Args[1]
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("could not get current directory: %v", err)
	}

	rootCmd := (&cli.CmdRoot{}).Command(cwd)

	err = os.MkdirAll(docDir, os.ModePerm)
	if err != nil {
		log.Fatalf("failed to create docs directory: %v", err)
	}

	err = GenReSTTreeCustom(rootCmd, filepath.Join(docDir), filePrepender, linkHandler)
	if err != nil {
		log.Fatalf("failed to generate documentation: %v", err)
	}
}
