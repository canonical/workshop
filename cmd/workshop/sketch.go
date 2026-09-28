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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/canonical/lxd/shared"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

type CmdSketch struct {
	root    *CmdRoot
	stash   bool
	restore bool
	eject   bool
	name    string
	remove  bool
	verbose bool
}

func (c *CmdSketch) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "sketch-sdk [<WORKSHOP>]",
		Args:    cobra.MaximumNArgs(1),
		Short:   "Customize a workshop",
		GroupID: GrpSketch,
		Long: `The command opens the sketch SDK template in the default text editor.
Add customizations by editing the template, then save and exit
the editor to apply the changes to the workshop.

The "--stash" and "--restore" options respectively stash the SDK,
reversing the changes, and quickly restore it to the workshop.

To make these customizations persistent,
run "workshop sketch-sdk --eject".
This saves the SDK definition under .workshop/ in the project directory,
so it can be committed to your repository.

The sketch SDK is intended for experiments and prototyping iterations.

Notes:

- You can only have one sketch SDK per workshop at a time.

- Run "workshop info" to list all SDKs currently installed
  in the workshop, including the sketch SDK if present.
`,
		Example: `
Edit the sketch SDK definition for the "nimble" workshop
and apply it after saving by automatically refreshing the workshop:
$ workshop sketch-sdk nimble

Save the sketch SDK for the "nimble" workshop
as a project SDK named "tools":
$ workshop sketch-sdk nimble --eject --name tools

Stash the sketch SDK, temporarily reverting the changes in the workshop:
$ workshop sketch-sdk nimble --stash`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopName([]string{"Ready", "Waiting"}),
	}

	cmd.Flags().BoolVar(&c.stash, "stash", false, "Stash the sketch SDK and remove it from the workshop.")
	cmd.Flags().BoolVar(&c.restore, "restore", false, "Return the previously stashed SDK to the workshop.")
	cmd.Flags().BoolVar(&c.eject, "eject", false, "Promote the sketch SDK to an in-project SDK.")
	cmd.Flags().StringVar(&c.name, "name", "", "Name for the ejected SDK.")
	cmd.Flags().BoolVar(&c.remove, "remove", false, "Remove the sketch SDK from the workshop.")
	cmd.Flags().BoolVar(&c.verbose, "verbose", false, "Combine stdout and stderr output from hooks.")

	_ = cmd.RegisterFlagCompletionFunc("name", cmdutil.CompleteChoices())

	cmd.MarkFlagsMutuallyExclusive("stash", "restore", "eject", "remove")

	return cmd
}

var sketchTemplate = `# For details: https://ubuntu.com/workshop/docs/explanation/sdks/concepts/#sketch-sdk
name: sketch
hooks:
  # Use 'setup-base' to customize the base image
  setup-base: |
    # apt-get update
    # apt-get install build-essential

  # Use 'setup-project' for project-specific logic, after plugs and slots are connected
  setup-project: |
    # uv sync


plugs:
  # Use this configuration to mount a host directory to the workshop
  # models:
  #   interface: mount
  #   workshop-target: /home/workshop/models


slots:
  # Use this configuration to expose a port, then add a corresponding system SDK plug
  # More details: https://ubuntu.com/workshop/docs/how-to/customize-workshops/forward-ports/
  # my-web-server:
  #   interface: tunnel
  #   endpoint: 8080
`

var (
	runTextEditor = shared.TextEditor
)

func stashSketch(sketchdir, stashdir string) (*revert.Reverter, error) {
	recs, err := os.ReadDir(sketchdir)
	if err != nil && !osutil.IsDirNotExist(err) {
		return nil, err
	}
	if len(recs) == 0 {
		// Nothing to do.
		return nil, errors.New(`"sketch" SDK not found`)
	}

	// Ensure stashdir exists but is empty.
	if err := clearStash(stashdir); err != nil {
		return nil, err
	}

	reverter := revert.New()
	defer reverter.Fail()

	if err := osutil.Exchange(sketchdir, stashdir); err != nil {
		return nil, err
	}
	reverter.Add(func() { _ = osutil.Exchange(stashdir, sketchdir) })

	if err := os.Remove(sketchdir); err != nil {
		return nil, err
	}
	reverter.Add(func() { _ = os.MkdirAll(sketchdir, 0755) })

	clone := reverter.Clone()
	reverter.Success()
	return clone, nil
}

func clearStash(stashdir string) error {
	temp, err := os.MkdirTemp(filepath.Dir(stashdir), "stash-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := os.Chmod(temp, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(stashdir, 0755); err != nil {
		return err
	}
	return osutil.Exchange(stashdir, temp)
}

func restoreSketch(sketchdir, stashdir string) error {
	recs, err := os.ReadDir(stashdir)
	if err != nil && !osutil.IsDirNotExist(err) {
		return err
	}
	if len(recs) == 0 || osutil.IsDirNotExist(err) {
		// Nothing in stored.
		return errors.New(`stashed "sketch" SDK not found`)
	}

	// We don't overwrite current sketch SDK as opposed to overwriting stashed
	// sketch SDK.
	if _, err = os.Stat(sketchdir); err == nil {
		return errors.New(`"sketch" SDK exists; run "workshop sketch-sdk --remove" to remove it from the workshop`)
	}

	return osutil.Rename(stashdir, sketchdir)
}

func (c *CmdSketch) inferSdkName(project string) error {
	inferred := false
	if c.name == "" {
		c.name = filepath.Base(project)
		inferred = true
	}

	err := sdk.ValidateName(c.name)
	if err != nil && inferred {
		err = fmt.Errorf("flag --name required: %w", err)
	}
	return err
}

func ejectSketch(project, sketchdir string, name string) (*revert.Reverter, error) {
	target := workshop.ProjectSdkPath(project, name)
	if osutil.FileExists(target) {
		return nil, &os.PathError{Op: "mkdir", Path: target, Err: os.ErrExist}
	}

	content, err := os.ReadFile(filepath.Join(sketchdir, "sdk.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(`"sketch" SDK not found`)
	} else if err != nil {
		return nil, err
	}
	file, err := sdk.ParseSketchYaml(bytes.NewReader(content))
	if err != nil {
		return nil, parseSketchYamlUserError(err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	projectFile := file
	projectFile.Name = name

	if err := sketchToProjectSdk(&document, projectFile.Name); err != nil {
		return nil, err
	}
	// This won't be cleaned up on failure. In most cases, the user
	// is likely to retry the eject after fixing the underlying issue.
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}

	reverter := revert.New()
	defer reverter.Fail()

	temp, err := os.MkdirTemp(filepath.Dir(target), name+"-*")
	if err != nil {
		return nil, err
	}
	reverter.Add(func() { _ = os.RemoveAll(temp) })

	f, err := os.OpenFile(filepath.Join(temp, "sdk.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := writeHooks(temp, file); err != nil {
		return nil, err
	}

	if err := os.Rename(temp, target); err != nil {
		return nil, err
	}

	reverter.Success()

	ejectReverter := revert.New()
	ejectReverter.Add(func() { _ = os.RemoveAll(target) })
	return ejectReverter, nil
}

func sketchToProjectSdk(document *yaml.Node, name string) error {
	var nodes struct {
		Name  NodeRef `yaml:"name"`
		Hooks NodeRef `yaml:"hooks"`
	}
	err := document.Decode(&nodes)
	if err != nil {
		return err
	}

	nodes.Name.Node.Value = name

	if nodes.Hooks.Node != nil {
		RemoveNodes(document, nodes.Hooks.Node)
	}

	return nil
}

func hideSketch(sketchdir string) (string, *revert.Reverter, error) {
	_, err := os.Stat(sketchdir)
	if err != nil && !osutil.IsDirNotExist(err) {
		return "", nil, err
	}
	if osutil.IsDirNotExist(err) {
		// Nothing to do.
		return "", nil, fmt.Errorf(`"sketch" SDK not found`)
	}

	reverter := revert.New()
	defer reverter.Fail()

	temp, err := os.MkdirTemp(filepath.Dir(sketchdir), "sketch-*")
	if err != nil {
		return "", nil, err
	}
	reverter.Add(func() { _ = os.RemoveAll(temp) })
	if err := os.Chmod(temp, 0755); err != nil {
		return "", nil, err
	}

	if err := osutil.Exchange(sketchdir, temp); err != nil {
		return "", nil, err
	}
	reverter.Add(func() { _ = osutil.Exchange(temp, sketchdir) })

	clone := reverter.Clone()
	reverter.Success()
	return temp, clone, nil
}

func handleSketchRefreshErrors(wp string, chg *client.Change, err error) error {
	switch {
	case client.IsNoUpdatesAvailable(err):
	case errors.Is(err, errUndone):
		fmt.Fprintf(Stdout, "%q sketch refresh aborted\n", wp)
	case errors.Is(err, errWaitOnError):
		return fmt.Errorf(`
cannot sketch %[1]q; paused
To view details: "workshop tasks %[2]s"

To abort and undo: "workshop refresh --abort %[1]s"
Otherwise, resolve the error, then run "workshop refresh --continue %[1]s"`[1:],
			wp,
			chg.ID,
		)
	default:
		return fmt.Errorf("%v\n%s sketch refresh aborted", err, wp)
	}
	return nil
}

func (c *CmdSketch) Run(cmd *cobra.Command, av []string) error {
	if c.name != "" && !c.eject {
		return errors.New("flag --name requires --eject")
	}

	cli, err := c.root.client()
	if err != nil {
		return err
	}
	p, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	var wp *client.Workshop
	if len(av) > 0 {
		wp, err = cli.Workshop(p.Id, av[0])
		if err != nil {
			return err
		}
	} else {
		wp, err = cli.SingleWorkshop(p)
		if err != nil {
			return err
		}
	}

	// Ensure that the workshop is Ready, aborting previous sketch if necessary.
	if wp.Status == "Waiting" {
		fmt.Fprintf(Stdout, "Reverting incomplete change for %q...\n", wp.Name)
		cmdabort := &CmdRefresh{root: c.root, Abort: true}
		// Skip the Undone status as this is desired; we will continue with the
		// regular sketch SDK flow afterwards.
		chg, err := cmdabort.RunRefresh(cli, p, []string{wp.Name})
		if err != nil && !errors.Is(err, errUndone) {
			return handleSketchRefreshErrors(wp.Name, chg, err)
		}
	} else if wp.Status != "Ready" {
		return fmt.Errorf(`cannot sketch: workshop %q currently %q, must be "Ready"`, wp.Name, wp.Status)
	}

	user, env, err := osutil.CurrentUserAndEnv()
	if err != nil {
		return err
	}

	userDataDir := workshop.UserDataRootDir(user.HomeDir, env)
	sketchdir := workshop.SketchSdkCurrent(userDataDir, p.Id, wp.Name)

	if c.stash {
		stashdir := workshop.SketchSdkStash(userDataDir, p.Id, wp.Name)

		reverter, err := stashSketch(sketchdir, stashdir)
		if err != nil {
			return fmt.Errorf("cannot stash: %w", err)
		}
		defer reverter.Fail()

		cmdrefresh := &CmdRefresh{root: c.root}
		cmdrefresh.verbose = c.verbose
		chg, err := cmdrefresh.RunRefresh(cli, p, []string{wp.Name})
		if err != nil {
			// Refresh failed, revert the stash operation so a possible subsequent
			// refresh won't fail due to the lack of a sketch SDK definition.
			return handleSketchRefreshErrors(wp.Name, chg, err)
		}
		fmt.Fprintf(Stdout, "%q sketch stashed\n", wp.Name)
		reverter.Success()
		return nil
	}

	if c.restore {
		cmdrefresh := &CmdRefresh{root: c.root}
		cmdrefresh.WaitOnError = true
		cmdrefresh.verbose = c.verbose

		stashdir := workshop.SketchSdkStash(userDataDir, p.Id, wp.Name)

		if err = restoreSketch(sketchdir, stashdir); err != nil {
			return fmt.Errorf("cannot restore: %w", err)
		}

		// Run refresh with the stashed sketch SDK. We do not revert dirs exchange
		// on a failed refresh here as it is run with the content from "stored"
		// and with --wait-on-error. Hence, there is always a possibility to
		// run 'workshop refresh --abort' and 'workshop sketch-sdk --stash' to restore the
		// original stash content.
		chg, err := cmdrefresh.RunRefresh(cli, p, []string{wp.Name})
		if err != nil {
			return handleSketchRefreshErrors(wp.Name, chg, err)
		}
		fmt.Fprintf(Stdout, "%q sketch restored\n", wp.Name)
		return nil
	}

	var ejectReverter *revert.Reverter
	if c.eject {
		if err := c.inferSdkName(p.Path); err != nil {
			return fmt.Errorf("cannot eject: %w", err)
		}

		ejectReverter, err = ejectSketch(p.Path, sketchdir, c.name)
		if err != nil {
			return fmt.Errorf("cannot eject: %w", err)
		}
		defer ejectReverter.Fail()
	}

	if c.eject || c.remove {
		temp, reverter, err := hideSketch(sketchdir)
		if err != nil {
			return fmt.Errorf("cannot remove: %w", err)
		}
		defer reverter.Fail()

		cmdrefresh := &CmdRefresh{root: c.root}
		cmdrefresh.verbose = c.verbose
		chg, err := cmdrefresh.RunRefresh(cli, p, []string{wp.Name})
		if err != nil {
			return handleSketchRefreshErrors(wp.Name, chg, err)
		}

		reverter.Success()
		if ejectReverter != nil {
			ejectReverter.Success()
		}
		_ = os.RemoveAll(temp)

		if c.remove {
			fmt.Fprintf(Stdout, "%q sketch removed\n", wp.Name)
		} else {
			fmt.Fprintf(Stdout, "Ejected %q sketch to %q.\n", wp.Name, workshop.ProjectSdkPath("", c.name))
			fmt.Fprintf(Stdout, "To use it, add %q to the SDK list and run %q\n", "project-"+c.name, "workshop refresh "+wp.Name)
		}

		return nil
	}

	if err = editSketchSdk(sketchdir); err != nil {
		return fmt.Errorf("cannot sketch: %w", err)
	}

	cmdrefresh := &CmdRefresh{root: c.root}
	cmdrefresh.WaitOnError = true
	cmdrefresh.verbose = c.verbose

	chg, err := cmdrefresh.RunRefresh(cli, p, []string{wp.Name})
	if err != nil {
		return handleSketchRefreshErrors(wp.Name, chg, err)
	}
	fmt.Fprintf(Stdout, "%q sketch refreshed\n", wp.Name)
	return nil
}

func editSketchSdk(sketchdir string) error {
	content, err := os.ReadFile(filepath.Join(sketchdir, "sdk.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(sketchdir, 0755); err != nil {
			return err
		}

		content = []byte(sketchTemplate)
	} else if err != nil {
		return err
	}

	temp, err := os.MkdirTemp(filepath.Dir(sketchdir), "current-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := os.Chmod(temp, 0755); err != nil {
		return err
	}

	target := filepath.Join(temp, "sdk.yaml")
	if err := writeSketchSdk(target, content); err != nil {
		return err
	}
	content, err = runTextEditor(target, content)
	if err != nil {
		return err
	}
	file, err := sdk.ParseSketchYaml(bytes.NewReader(content))
	if err != nil {
		// If parsing failed, we don't want to refresh but we do want to
		// remember the user's edits for next time.
		_ = osutil.Exchange(temp, sketchdir)
		return parseSketchYamlUserError(err)
	}

	if err := writeHooks(temp, file); err != nil {
		// If writeHooks failed, we don't want to refresh but we do want to
		// remember the user's edits for next time.
		_ = osutil.Exchange(temp, sketchdir)
		return err
	}

	return osutil.Exchange(temp, sketchdir)
}

// parseSketchYamlUserError converts structured [sdk.ParseSketchYaml] errors
// into messages that tell sketch SDK users what to fix in their YAML. Use it
// when reporting parse failures from sketch command flows, where raw validator
// errors can be too generic or expose SDK-internal terms.
func parseSketchYamlUserError(err error) error {
	var (
		invalidHook  sdk.InvalidSDKHookNameError
		unknownField *sdk.UnknownYamlFieldsError
	)

	switch {
	case errors.Is(err, sdk.ErrorInvalidSDKName):
		return fmt.Errorf(
			"sketch SDK YAML must keep name set to %q",
			sdk.Sketch,
		)
	case errors.As(err, &invalidHook):
		return fmt.Errorf(
			"sketch SDK YAML contains unsupported hook %q; supported hooks: %s",
			string(invalidHook),
			strings.Join(sdk.AllowedHooks, ", "),
		)
	case errors.As(err, &unknownField):
		parts := make([]string, 0, len(unknownField.Fields))
		for name, field := range unknownField.Fields {
			parts = append(
				parts,
				fmt.Sprintf(
					"%q at line %d, column %d",
					name,
					field.Line,
					field.Column,
				),
			)
		}

		return fmt.Errorf(
			"sketch SDK YAML contains unknown fields: %s",
			strings.Join(parts, ", "),
		)
	default:
		return fmt.Errorf("could not parse sketch SDK YAML: %v", err)
	}
}

func writeSketchSdk(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

func writeHooks(sdkdir string, file sdk.SketchSDKYaml) error {
	hooksdir := filepath.Join(sdkdir, "hooks")
	if len(file.Hooks) > 0 {
		if err := os.MkdirAll(hooksdir, 0755); err != nil {
			return err
		}
	}

	for hook, script := range file.Hooks {
		hookpath := filepath.Join(hooksdir, hook)
		if !strings.HasSuffix(script, "\n") {
			script += "\n"
		}
		err := os.WriteFile(hookpath, []byte(script), 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

type CmdSketches struct {
	cmdutil.ColorMixin
	root      *CmdRoot
	noHeaders bool
}

func (c *CmdSketches) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "sketches",
		Args:    cobra.ExactArgs(0),
		Short:   "List project sketch SDKs",
		GroupID: GrpSketch,
		Long: `
This command enumerates all sketches in the project, printing a compact list:

- Project:  absolute pathname of the project

- Workshop: workshop name, as set in its definition

- Rev:      sketch SDK revision, if present

- Notes:    current, stashed, or both
`,
		Example: `
List the sketches in the current project directory:
$ workshop sketches`,
		RunE: c.Run,
	}

	cmd.PersistentFlags().BoolVar(&c.noHeaders, "no-headers", false, "Hide table headers.")

	return cmd
}

func (c *CmdSketches) Run(cmd *cobra.Command, _ []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	w := tabWriter()

	p, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	wps, _, err := cli.List(&client.ListOptions{ProjectId: p.Id})
	if err != nil {
		return err
	}

	user, env, err := osutil.CurrentUserAndEnv()
	if err != nil {
		return err
	}

	userDataDir := workshop.UserDataRootDir(user.HomeDir, env)

	var entries []*stashInfo
	for _, wp := range wps {
		entry := stashEntry(userDataDir, wp, p)
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	var maxRev int
	if !c.noHeaders {
		maxRev = len("REV")
	}
	for _, entry := range entries {
		maxRev = max(maxRev, len(entry.rev))
	}
	if !c.noHeaders && len(entries) > 0 {
		esc := c.GetEscapes()
		fmt.Fprintf(w, "%sWORKSHOP\t%*s\tNOTES%s\n", esc.Bold, maxRev, "REV", esc.End)
	}
	for _, entry := range entries {
		fmt.Fprintf(w, "%s\t%*s\t%s\n", entry.workshop, maxRev, entry.rev, entry.notes)
	}

	w.Flush()

	return nil
}

type stashInfo struct {
	workshop string
	rev      string
	notes    string
}

func stashEntry(userDataDir string, w *client.WorkshopInfo, p *client.Project) *stashInfo {
	rev := "-"
	notes := ""
	exists := false
	idx := slices.IndexFunc(w.Sdks, func(s *client.Sdk) bool { return sdk.IsSketch(s.Name) })
	if idx != -1 {
		info := w.Sdks[idx]
		rev = info.Revision
		notes = "current"
		exists = true
	}

	stashdir := workshop.SketchSdkStash(userDataDir, p.Id, w.Name)

	if osutil.IsDir(stashdir) {
		if len(notes) > 0 {
			notes += ","
		}
		notes += "stashed"
		exists = true
	}

	if !exists {
		return nil
	}

	return &stashInfo{w.Name, rev, notes}
}
