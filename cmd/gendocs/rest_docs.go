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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// printOptionsReST updates the formatting of command options.
func printOptionsReST(buf *bytes.Buffer, cmd *cobra.Command, name string) error {
	flags := cmd.NonInheritedFlags()
	flags.SetOutput(buf)
	if flags.HasAvailableFlags() {
		buf.WriteString(`
Options
~~~~~~~

.. code-block:: console

`)
		flags.PrintDefaults()
		buf.WriteString("\n")
	}
	return nil
}

// GenReSTCustom creates custom reStructured Text output with the specified formatting.
func GenReSTCustom(cmd *cobra.Command, w io.Writer, linkHandler func(string, string) string) error {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	buf := new(bytes.Buffer)
	name := cmd.CommandPath()

	short := cmd.Short
	long := cmd.Long
	if len(long) == 0 {
		long = short
	}
	ref := "ref_" + strings.ReplaceAll(name, " ", "_")

	buf.WriteString(fmt.Sprintf(`.. _%s:

%s
%s

%s

Synopsis
~~~~~~~~

%s

`,
		ref, name, strings.Repeat("-", len(name)), short, long))

	if cmd.Runnable() {
		buf.WriteString(".. code-block:: console\n\n")
		buf.WriteString(fmt.Sprintf("   %s\n\n", cmd.UseLine()))
	}

	if len(cmd.Example) > 0 {
		buf.WriteString(`
Examples
~~~~~~~~

.. code-block:: console

		`)
		buf.WriteString(fmt.Sprintf("%s\n\n", indentString(cmd.Example, "  ")))
	}

	if err := printOptionsReST(buf, cmd, name); err != nil {
		return err
	}

	_, err := buf.WriteTo(w)
	return err
}

func loadFileContent(filename string) (string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filename, err)
	}
	return string(content), nil
}

// GenReSTTreeCustom generates RST documentation and creates an index.rst file.
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

	// Create index.rst with template content and includes for each command file
	indexPath := filepath.Join(dir, "index.rst")
	indexFile, err := os.Create(indexPath)
	if err != nil {
		return err
	}
	defer indexFile.Close()

	// Read and write header for the index.rst file
	headerContent, err := loadFileContent("header.rst")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading header: %v\n", err)
		return err
	}

	if _, err := indexFile.WriteString(headerContent); err != nil {
		return err
	}

	// Include each command's RST file
	for _, file := range files {
		includeDirective := fmt.Sprintf(".. include:: %s\n\n", file)
		if _, err := indexFile.WriteString(includeDirective); err != nil {
			return err
		}
	}

	// Read and write footer for the index.rst file
	footerContent, err := loadFileContent("footer.rst")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading footer: %v\n", err)
		return err
	}

	if _, err := indexFile.WriteString(footerContent); err != nil {
		return err
	}

	return nil
}

// indentString adapted from: https://github.com/kr/text/blob/main/indent.go
func indentString(s, p string) string {
	var res []byte
	b := []byte(s)
	prefix := []byte(p)
	bol := true
	for _, c := range b {
		if bol && c != '\n' {
			res = append(res, prefix...)
		}
		res = append(res, c)
		bol = c == '\n'
	}
	return string(res)
}
