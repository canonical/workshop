package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/canonical/lxd/shared"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/client"
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
}

func (c *CmdSketch) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "sketch-sdk [--stash|--restore|--eject|--remove] [<WORKSHOP>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Edit the sketch SDK and graft it onto the workshop",
		Long: `
This opens the 'sketch' SDK definition in the default text editor,
enabling rapid experiments and tweaks at the SDK level.

Saving the definition and exiting the editor causes a refresh,
which installs the configured 'sketch' SDK in the workshop.

The '--stash' and '--restore' options respectively stash the SDK,
reversing the changes, and quickly restore it to the workshop.
The '--eject' option moves the SDK definition into the project directory,
so it can be added to multiple workshops or shared with others.
The '--remove' option removes the SDK permanently.

Notes:

- The 'sketch' SDK doesn't appear in the workshop definition
  and cannot include build-time data such as parts.

- In addition to hooks, the 'sketch' SDK can use interfaces
  by defining plugs and slots.
`,
		Example: `
Edit the sketch SDK definition for the 'nimble' workshop
and apply it after saving by automatically refreshing the workshop:
$ workshop sketch-sdk nimble

The name is optional if the project has only one workshop:
$ workshop sketch-sdk

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

	cmd.MarkFlagsMutuallyExclusive("stash", "restore", "eject", "remove")

	return cmd
}

var sketchTemplate = `# Sketch SDK for %s
# Sketch SDK provides local customisation of this specific workshop.

# To read more about the sketch SDK, its and syntax, see:
# https://canonical-workshop.readthedocs-hosted.com/en/latest/explanation/sdks/sdks/#sketch-sdk
name: sketch
hooks:
  # EXAMPLE: setup-base runs once at workshop launch, use it to install some packages.
  # setup-base: |
    # apt-get update
    # apt-get install PACKAGE...
    # snap install SNAP...
  # EXAMPLE: setup-project runs after connecting plugs and slots, use it to install the project environment.
  # setup-project: |
    # go mod download
    # uv sync
  # EXAMPLE: check-health runs after all SDK setup completes, call 'workshopctl set-health okay' for OK.
  # check-health: |
    # if CHECK_HEALTH_COMMAND ; then
    #   workshopctl set-health okay
    # else
    #   workshopctl set-health --code=installation-failed error "Installation failed"
    # fi
plugs:
  # EXAMPLE: forward your SSH agent into the workshop enabling 'git push' inside the workshop.
  # ssh-agent:
  #   interface: ssh-agent
  # EXAMPLE: expose well-known config file locations to the workshop
  # vs-code-settings:
  #   interface: mount
  #   workshop-target: /home/workshop/.config/Code/User
slots:
  # EXAMPLE: expose SDK services to the host
  # dashboard:
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
		return errors.New(`"sketch" SDK exists; run 'workshop sketch-sdk --remove' to remove it from the workshop`)
	}

	return osutil.Rename(stashdir, sketchdir)
}

func (c *CmdSketch) inferSdkName(project string) error {
	inferred := false
	if c.name == "" {
		c.name = filepath.Base(project)
		inferred = true
	}

	if sdk.SdkName.MatchString(c.name) {
		return nil
	}

	err := fmt.Errorf("invalid SDK name %q", c.name)
	if inferred {
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
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}

	var rec workshop.SdkRecord
	if err := document.Decode(&rec); err != nil {
		return nil, err
	}
	rec.Name = name

	if err := sketchToProjectSdk(&document, name); err != nil {
		return nil, err
	}
	if err := validateProjectSdk(&document, &rec); err != nil {
		return nil, err
	}

	// This won't be cleaned up on failure. In most cases the user
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
	if err := writeHooks(temp, rec); err != nil {
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
	if err := document.Decode(&nodes); err != nil {
		return err
	}

	if nodes.Name.Node == nil {
		return errors.New(`"sketch" SDK name not found`)
	}
	nodes.Name.Node.Value = name

	if nodes.Hooks.Node != nil {
		RemoveNodes(document, nodes.Hooks.Node)
	}

	return nil
}

func validateProjectSdk(document *yaml.Node, expected *workshop.SdkRecord) error {
	var actual workshop.SdkRecord
	if err := document.Decode(&actual); err != nil {
		return err
	}

	if actual.Name != expected.Name {
		return fmt.Errorf("internal error: sketch SDK renamed %q (expected %q)", actual.Name, expected.Name)
	}
	if !reflect.DeepEqual(expected.Plugs, actual.Plugs) {
		return errors.New("internal error: sketch SDK plugs not preserved")
	}
	if !reflect.DeepEqual(actual.Slots, actual.Slots) {
		return errors.New("internal error: sketch SDK slots not preserved")
	}
	if len(actual.Hooks) > 0 {
		return errors.New("internal error: hooks not ejected from sketch SDK")
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
		if err = cmdabort.RunRefresh(cli, p, []string{wp.Name}); err != nil {
			return err
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
		if err = cmdrefresh.RunRefresh(cli, p, []string{wp.Name}); err != nil {
			// Refresh failed, revert the stash operation so a possible subsequent
			// refresh won't fail due to the lack of a sketch SDK definition.
			return err
		}
		fmt.Fprintf(Stdout, "%q sketch stashed\n", wp.Name)
		reverter.Success()
		return nil
	}

	if c.restore {
		cmdrefresh := &CmdRefresh{root: c.root}
		cmdrefresh.WaitOnError = true

		stashdir := workshop.SketchSdkStash(userDataDir, p.Id, wp.Name)

		if err = restoreSketch(sketchdir, stashdir); err != nil {
			return fmt.Errorf("cannot restore: %w", err)
		}

		// Run refresh with the stashed sketch SDK. We do not revert dirs exchange
		// on a failed refresh here as it is run with the content from "stored"
		// and with --wait-on-error. Hence, there is always a possibility to
		// workshop refresh --abort and workshop sketch-sdk --stash to restore the
		// original stash content.
		if err = cmdrefresh.RunRefresh(cli, p, []string{wp.Name}); err != nil {
			return err
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
		if err = cmdrefresh.RunRefresh(cli, p, []string{wp.Name}); err != nil {
			return err
		}

		reverter.Success()
		if ejectReverter != nil {
			ejectReverter.Success()
		}
		_ = os.RemoveAll(temp)

		if c.remove {
			fmt.Fprintf(Stdout, "%q sketch removed\n", wp.Name)
		} else {
			fmt.Fprintf(Stdout, "%q sketch ejected to %q\n", wp.Name, workshop.ProjectSdkPath("", c.name))
			fmt.Fprintf(Stdout, "To use it, add %q to the list of SDKs and run 'workshop refresh %s'\n", "project-"+c.name, wp.Name)
		}

		return nil
	}

	if err = editSketchSdk(sketchdir, wp.Path); err != nil {
		return fmt.Errorf("cannot sketch: %w", err)
	}

	cmdrefresh := &CmdRefresh{root: c.root}
	cmdrefresh.WaitOnError = true

	if err = cmdrefresh.RunRefresh(cli, p, []string{wp.Name}); err != nil {
		return err
	}
	fmt.Fprintf(Stdout, "%q sketch refreshed\n", wp.Name)
	return nil
}

func editSketchSdk(sketchdir, workshopFile string) error {
	content, err := os.ReadFile(filepath.Join(sketchdir, "sdk.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(sketchdir, 0755); err != nil {
			return err
		}

		// Format sketch SDK template header.
		content = fmt.Appendf(nil, sketchTemplate, workshopFile)
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
	if err := writeSketchHooks(temp, content); err != nil {
		// If writeSketchHooks failed, we don't want to refresh
		// but we do want to remember the user's edits for next time.
		_ = osutil.Exchange(temp, sketchdir)
		return err
	}

	return osutil.Exchange(temp, sketchdir)
}

func writeSketchSdk(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

func writeSketchHooks(sketchdir string, content []byte) error {
	var rec workshop.SdkRecord
	if err := yaml.Unmarshal(content, &rec); err != nil {
		return err
	}

	if !sdk.IsSketch(rec.Name) {
		return fmt.Errorf("SDK must be named %q (now: %q)", sdk.Sketch, rec.Name)
	}

	return writeHooks(sketchdir, rec)
}

func writeHooks(sdkdir string, rec workshop.SdkRecord) error {
	hooksdir := filepath.Join(sdkdir, "hooks")
	if len(rec.Hooks) > 0 {
		if err := os.MkdirAll(hooksdir, 0755); err != nil {
			return err
		}
	}

	for _, hook := range []string{"setup-base", "setup-project", "save-state", "restore-state", "check-health"} {
		hookpath := filepath.Join(hooksdir, hook)
		if script := rec.Hooks[hook]; len(script) > 0 {
			if !strings.HasSuffix(script, "\n") {
				script += "\n"
			}
			if err := os.WriteFile(hookpath, []byte(script), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

type CmdSketches struct {
	root *CmdRoot
}

func (c *CmdSketches) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "sketches",
		Args:  cobra.ExactArgs(0),
		Short: "List sketches",
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

	var entries []string
	for _, wp := range wps {
		entry := stashEntry(userDataDir, wp, p)
		if entry != nil {
			entries = append(entries, strings.Join(entry, "\t"))
		}
	}

	if entries != nil {
		fmt.Fprintln(w, "Project\tWorkshop\tRev\tNotes")
		fmt.Fprintln(w, strings.Join(entries, "\n"))
	}

	w.Flush()

	return nil
}

func stashEntry(userDataDir string, w *client.WorkshopInfo, p *client.Project) []string {
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

	return []string{contractHomeDirectory(p.Path), w.Name, rev, notes}
}
