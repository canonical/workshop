package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/workshop"
)

var defaultBase = "ubuntu@24.04"

type CmdInit struct {
	root *CmdRoot
	sdks []string
	base string
	vm   bool
}

func (c *CmdInit) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "init <NAME>",
		Args:    cobra.ExactArgs(1),
		Short:   "Create a new workshop definition in the project directory",
		GroupID: GrpCRUD,
		Long: `
Create a new workshop definition file in the project's .workshop/ directory.

The NAME argument sets the workshop name. The command creates a named
workshop file at .workshop/<NAME>.yaml. This fails if a workshop with
the same name already exists.

The supported bases are ubuntu@20.04, ubuntu@22.04, ubuntu@24.04, and ubuntu@26.04.

SDKs are specified as a comma-separated list. Each SDK entry can optionally
include a channel using the <NAME>/<CHANNEL> syntax (e.g., "go/1.26/stable").
`,
		Example: `
Create a workshop called "dev" with the Go and UV SDKs:
$ workshop init dev --sdks go,uv

Create a workshop with a specific SDK channel:
$ workshop init dev --sdks go/1.26/stable

Create a workshop using a specific base:
$ workshop init dev --base ubuntu@22.04 --sdks go`,
		RunE: c.Run,
	}

	cmd.Flags().StringSliceVar(&c.sdks, "sdks", nil, `Comma-separated list of SDKs (e.g., "go,uv/latest/stable").`)
	cmd.Flags().StringVar(&c.base, "base", defaultBase, "Base image for the workshop.")
	cmd.Flags().BoolVar(&c.vm, "vm", false, "Use a virtual machine instead of a container.")

	cmd.MarkFlagsMutuallyExclusive("sdks", "vm")

	return cmd
}

func (c *CmdInit) Run(cmd *cobra.Command, args []string) error {
	projectDir := c.root.project()
	name := args[0]

	sdks, err := parseSdkArgs(c.sdks)
	if err != nil {
		return err
	}

	confinement := workshop.ConfinementContainer
	if c.vm {
		confinement = workshop.ConfinementVirtualMachine
	}

	wfile := &workshop.File{
		Name: name,
		Base: c.base,
		Sdks: sdks,
	}

	if err := workshop.ValidateFile(wfile); err != nil {
		return err
	}

	// TODO: include confinement in the validation. We skip validation here
	// because we might not have the experimental flag in the environment.
	wfile.Confinement = confinement

	if err := ensureCanCreate(projectDir, name); err != nil {
		return err
	}

	path, err := writeWorkshopFile(projectDir, wfile)
	if err != nil {
		return err
	}

	fmt.Fprintf(Stdout, "%q workshop created at %s\n", name, path)
	return nil
}

// parseSdkArgs converts a list of strings like ["go/1.26/stable", "python"]
// into SdkRecord entries.
func parseSdkArgs(args []string) ([]workshop.SdkRecord, error) {
	var sdks []workshop.SdkRecord
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		yamlName, channel, _ := strings.Cut(arg, "/")
		name, source, err := workshop.ParseSdkName(yamlName)
		if err != nil {
			return nil, err
		}

		sdks = append(sdks, workshop.SdkRecord{
			Name:    name,
			Channel: channel,
			Source:  source,
		})
	}

	return sdks, nil
}

// ensureCanCreate checks that no single-file workshop definition exists in the
// project root and that no named workshop with the same name already exists.
func ensureCanCreate(projectDir, name string) error {
	for _, fname := range workshop.Filenames {
		path := filepath.Join(projectDir, fname)
		if _, err := os.Stat(path); err == nil {
			dest := workshop.Directory + "/"
			if wname := workshopNameFromFile(path); wname != "" {
				dest = filepath.Join(workshop.Directory, wname+".yaml")
			}
			return fmt.Errorf("cannot init: %q already exists, move it to %s first to manage multiple workshops",
				fname, dest)
		}
	}

	path := workshop.Filepath(projectDir, name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("cannot init: %q workshop already exists at %q", name, path)
	}
	return nil
}

// workshopNameFromFile reads a workshop YAML file and returns its name field.
func workshopNameFromFile(path string) string {
	buf, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var partial struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(buf, &partial); err != nil {
		return ""
	}
	return partial.Name
}

// writeWorkshopFile creates the .workshop/ directory and writes the YAML file.
func writeWorkshopFile(projectDir string, wfile *workshop.File) (string, error) {
	dir := filepath.Join(projectDir, workshop.Directory)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create %q: %w", dir, err)
	}

	path := workshop.Filepath(projectDir, wfile.Name)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(wfile); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}

	if err := os.WriteFile(path, out.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("cannot write %q: %w", path, err)
	}

	return path, nil
}
