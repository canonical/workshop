package main

import (
	"fmt"
	"log"
	"os"

	"github.com/canonical/workshop/cmd/workshop/gendocs"
	"github.com/spf13/cobra"
)

type CmdDocs struct {
	root   *CmdRoot
	global bool
}

func (c *CmdDocs) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:    "generate-docs",
		Args:   cobra.RangeArgs(0, 1),
		Short:  "Generate workshop reference docs",
		Hidden: true,
		RunE:   c.Run,
	}

	return cmd
}

func filePrepender(filename string) string {
	return ""
}

func linkHandler(name, ref string) string {
	return fmt.Sprintf(":ref:`%s <%s>`", name, ref)
}

func (c *CmdDocs) Run(cmd *cobra.Command, av []string) error {
	docDir := "docs-gendocs"
	if len(av) > 1 {
		docDir = av[1]
	}

	err := os.MkdirAll(docDir, os.ModePerm)
	if err != nil {
		log.Fatalf("failed to create docs directory: %v", err)
	}

	err = gendocs.GenReSTTreeCustom(c.root.Command(c.root.project), docDir, filePrepender, linkHandler)
	if err != nil {
		log.Fatalf("failed to generate documentation: %v", err)
	}
	return nil
}
