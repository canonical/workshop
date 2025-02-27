package main

import (
	"fmt"
	"log"
	"os"

	"github.com/canonical/gencodo"
	"github.com/spf13/cobra"
)

type CmdDocs struct {
	root *CmdRoot
}

func (c *CmdDocs) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:    "generate-docs",
		Args:   cobra.MaximumNArgs(1),
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
	if len(av) > 0 {
		docDir = av[0]
	}

	err := os.MkdirAll(docDir, os.ModePerm)
	if err != nil {
		log.Fatalf("failed to create docs directory: %v", err)
	}

	err = gencodo.GenReSTTreeCustom(c.root.Command(c.root.project), docDir, filePrepender, linkHandler)
	if err != nil {
		log.Fatalf("failed to generate documentation: %v", err)
	}
	return nil
}
