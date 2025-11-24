package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/canonical/gencodo"
	"github.com/spf13/cobra"
)

//go:embed doc-templates/*
var templates embed.FS

type CmdDocs struct {
	root *CmdRoot
}

func (c *CmdDocs) Command() *cobra.Command {
	cmd := &cobra.Command{
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

	indexTemplate, err := templates.ReadFile("doc-templates/cli.rst")
	if err != nil {
		return err
	}
	singleCommandTemplate, err := templates.ReadFile("doc-templates/command.rst")
	if err != nil {
		return err
	}

	td := gencodo.TemplateInfo{
		IndexFileName:         "workshop.rst",
		IndexTemplate:         string(indexTemplate),
		SingleCommandTemplate: string(singleCommandTemplate),
	}

	err = gencodo.GenDocsTree(
		c.root.Command(),
		docDir,
		td,
		filePrepender,
		linkHandler,
	)
	if err != nil {
		log.Fatalf("failed to generate documentation: %v", err)
	}
	return nil
}
