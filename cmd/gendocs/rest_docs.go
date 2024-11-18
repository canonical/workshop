// Copyright 2013-2023 The Cobra Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"embed"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:embed cli.tmpl command.tmpl
var templates embed.FS

// FlagDetail holds details about a flag
type FlagDetail struct {
	Name         string
	Usage        string
	DefaultValue string
}

// GenReSTCustom creates custom reStructured Text output with the specified formatting.
func GenReSTCustom(cmd *cobra.Command, w io.Writer, linkHandler func(string, string) string) error {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	// Prepare data for the template
	name := cmd.CommandPath()

	short := cmd.Short
	long := cmd.Long
	if len(long) == 0 {
		long = short
	}
	ref := "ref_" + strings.ReplaceAll(name, " ", "_")

	// Compute the heading separator
	heading := strings.Repeat("-", len(name))

	// Collect flag details
	flags := cmd.NonInheritedFlags()
	var flagDetails []FlagDetail
	flags.VisitAll(func(flag *pflag.Flag) {
		flagDetails = append(flagDetails, FlagDetail{
			Name:         flag.Name,
			Usage:        flag.Usage,
			DefaultValue: flag.DefValue,
		})
	})

	// Prepare the template data
	data := struct {
		Ref         string
		CommandName string
		Short       string
		Long        string
		Synopsis    string
		Examples    string
		Flags       []FlagDetail
		Heading     string
	}{
		Ref:         ref,
		CommandName: name,
		Short:       short,
		Long:        long,
		Synopsis:    cmd.UseLine(),
		Examples:    cmd.Example,
		Flags:       flagDetails,
		Heading:     heading,
	}

	// Define the indent helper function
	funcMap := template.FuncMap{
		"indent": func(spaces int, ss ...string) string {
			padding := strings.Repeat(" ", spaces)
			var indentedStrings []string
			for _, s := range ss {
				indentedStrings = append(indentedStrings, padding+strings.ReplaceAll(s, "\n", "\n"+padding))
			}
			return strings.Join(indentedStrings, "\n")
		},
	}

	// Read and parse the template
	tmplContent, err := templates.ReadFile("command.tmpl")
	if err != nil {
		return err
	}

	tmpl, err := template.New("command").Funcs(funcMap).Parse(string(tmplContent))
	if err != nil {
		return err
	}

	// Render the template
	buf := new(bytes.Buffer)
	if err = tmpl.Execute(buf, data); err != nil {
		return err
	}

	_, err = buf.WriteTo(w)
	return err
}

// GenReSTTreeCustom generates RST documentation and creates an index.rst file using a template.
func GenReSTTreeCustom(cmd *cobra.Command, dir string, filePrepender func(string) string, linkHandler func(string, string) string) error {
	var files []string

	// Recursive function to generate documentation for each command
	var generateDocs func(*cobra.Command) error
	generateDocs = func(c *cobra.Command) error {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			return nil
		}

		// Generate docs for subcommands
		for _, subCmd := range c.Commands() {
			if err := generateDocs(subCmd); err != nil {
				return err
			}
		}

		// Generate RST file for the command
		basename := strings.ReplaceAll(c.CommandPath(), " ", "-") + ".rst"
		filename := filepath.Join(dir, basename)
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.WriteString(f, filePrepender(filename)); err != nil {
			return err
		}
		if err := GenReSTCustom(c, f, linkHandler); err != nil {
			return err
		}

		// Track generated files for index
		files = append(files, basename)
		return nil
	}

	// Generate docs for subcommands only
	for _, subCmd := range cmd.Commands() {
		if err := generateDocs(subCmd); err != nil {
			return err
		}
	}

	// Sort the RST files in alphabetical order
	sort.Strings(files)

	// Prepare data for the index template
	data := struct {
		Files []string
	}{
		Files: files,
	}

	// Read and parse the template
	templateContent, err := templates.ReadFile("cli.tmpl")
	if err != nil {
		return err
	}

	tmpl, err := template.New("index").Parse(string(templateContent))
	if err != nil {
		return err
	}

	// Create and write the index.rst file
	indexPath := filepath.Join(dir, "index.rst")
	indexFile, err := os.Create(indexPath)
	if err != nil {
		return err
	}
	defer indexFile.Close()

	if err := tmpl.Execute(indexFile, data); err != nil {
		return err
	}

	return nil
}
