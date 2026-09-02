// Copyright (c) 2026 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"fmt"

	"github.com/canonical/gencodo"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/cmd/internal/doctemplates"
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

func (c *CmdDocs) Run(cmd *cobra.Command, av []string) error {
	docDir := "docs-gendocs"
	if len(av) > 0 {
		docDir = av[0]
	}

	indexTemplate, err := doctemplates.ReadFile("workshop.rst")
	if err != nil {
		return err
	}
	singleCommandTemplate, err := doctemplates.ReadFile("command.rst")
	if err != nil {
		return err
	}

	td := gencodo.TemplateInfo{
		IndexFileName:         "workshop.rst",
		IndexTemplate:         string(indexTemplate),
		SingleCommandTemplate: string(singleCommandTemplate),
	}

	err = gencodo.GenRSTTree(
		c.root.Command(),
		docDir,
		td,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to generate documentation: %w", err)
	}
	return nil
}
