// Copyright (c) 2021 Canonical Ltd
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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/ptyutil"
	"github.com/canonical/workshop/internal/workshop"
)

type CmdExec struct {
	root  *CmdRoot
	flags ExecFlags
}

type CmdShell struct {
	root *CmdRoot
}

type CmdRun struct {
	root  *CmdRoot
	flags ExecFlags
}

type ExecFlags struct {
	WorkingDir     string
	Env            []string
	UserId         int
	GroupId        int
	Timeout        time.Duration
	Interactive    bool
	NonInteractive bool
}

type ExecArgs struct {
	workshop string
	implicit bool
	command  []string
	action   bool
}

var (
	shortExecHelp = "Run a command and wait for it to complete"
	longExecHelp  = `
The 'exec' subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept an 'exec' command, the workshop must be 'Ready' or 'Waiting'.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'exec' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'exec' subcommand from the command itself,
use shell syntax such as *--*.
This syntax is required if the workshop name is omitted.

Notes:

- To start a workshop before running commands in it, use 'workshop start'.

- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided.
`
)

var (
	shortShellHelp = "Start an interactive terminal session for the workshop"
	longShellHelp  = `
The 'shell' subcommand runs an interactive terminal session
in the specified workshop.

To accept a 'shell' command, the workshop must be 'Ready' or 'Waiting'.


Notes:

- To start a workshop before running a terminal session, use 'workshop start'.

- The subcommand is a shorthand for 'workshop exec';
  it launches the login shell for 'workshop',
  the default non-privileged user in a workshop.
`
)

var (
	shortRunHelp = "Run a workshop action and wait for it to complete"
	longRunHelp  = `
The 'run' subcommand runs an action specified in the workshop definition file,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept a 'run' command, the workshop must be 'Ready' or 'Waiting'.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'run' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'run' subcommand from the action and its arguments,
use shell syntax such as *--*.
This syntax is required if the workshop name is omitted
and the action takes one or more arguments.

Notes:

- To start a workshop before running actions in it, use 'workshop start'.

- You can set the working directory, environment variables, user and group ID
  for running the action in the workshop; reasonable defaults are provided.
`
)

var (
	shortActionsHelp = "List workshop actions"
	longActionsHelp  = `
This command enumerates all actions in the workshop, printing a YAML map.
`
)

func (c *CmdExec) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [flags] [<WORKSHOP>] [--] <COMMAND>...",
		Args:  maybeNameAndCommand,
		Short: shortExecHelp,
		Long:  longExecHelp,
		Example: `
Run the 'go build main.go' command under the 'nimble' workshop
in the current project directory:
$ workshop exec nimble go build main.go

A similar command that sets an environment variable and the working directory:
$ workshop exec --env GO111MODULE=off -w /project nimble go build -x

Run a custom interactive shell:
$ workshop exec -I nimble sh

The name is optional if the project has only one workshop
and a separator is provided:
$ workshop exec -I -- sh

Run a command as root (the default is 'workshop'):
$ workshop exec --uid 0 nimble id`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopName([]string{"Ready", "Waiting"}),
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().SetInterspersed(false)
	commonVars(cmd.Flags(), &c.flags)

	return cmd
}

func maybeNameAndCommand(cmd *cobra.Command, av []string) error {
	if cmd.ArgsLenAtDash() == 0 {
		// Workshop name is implicit if -- precedes all positional arguments
		return cobra.MinimumNArgs(1)(cmd, av)
	}

	argCount := len(av)
	if cmd.ArgsLenAtDash() < 0 && slices.Contains(av, "--") {
		argCount--
	}

	if argCount < 2 {
		return fmt.Errorf("requires at least 2 arg(s), only received %d", argCount)
	}
	return nil
}

func (c *CmdExec) Run(cmd *cobra.Command, av []string) error {
	args := &ExecArgs{}

	// Infer workshop name if first positional argument is --
	if cmd.ArgsLenAtDash() == 0 {
		args.implicit = true
		args.command = av
	} else {
		// Remove first -- if cobra didn't see it
		if cmd.ArgsLenAtDash() < 0 {
			if i := slices.Index(av, "--"); i >= 0 {
				av = slices.Delete(slices.Clone(av), i, i+1)
			}
		}

		args.workshop = av[0]
		args.command = av[1:]
	}

	return exec(c.root, &c.flags, args)
}

func (c *CmdShell) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell [<WORKSHOP>]",
		Args:  cobra.MaximumNArgs(1),
		Short: shortShellHelp,
		Long:  longShellHelp,
		Example: `
Open the default login shell of the 'workshop' user into the 'nimble' workshop
in the current project directory:
$ workshop shell nimble

The name is optional if the project has only one workshop:
$ workshop shell`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopName([]string{"Ready", "Waiting"}),
	}

	return cmd
}

func (c *CmdShell) Run(cmd *cobra.Command, av []string) error {
	args := &ExecArgs{command: []string{"bash"}}

	if len(av) > 0 {
		args.workshop = av[0]
	} else {
		args.implicit = true
	}

	flags := &ExecFlags{
		WorkingDir: "/project",
		UserId:     workshop.Uid,
		GroupId:    workshop.Gid,
	}

	return exec(c.root, flags, args)
}

func (c *CmdRun) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flags] [<WORKSHOP>] [--] <ACTION> <ARGUMENTS>...",
		Args:  maybeNameAndAction,
		Short: shortRunHelp,
		Long:  longRunHelp,
		Example: `
Run the 'build' action under the 'nimble' workshop
in the current project directory:
$ workshop run nimble build

A similar command that sets an environment variable and the working directory:
$ workshop run --env GO111MODULE=off -w /project nimble build

The workshop name is optional if the project only has one workshop:
$ workshop run build

Actions can accept arguments,
if a separator or a workshop name is provided:
$ workshop run -- build --debug
`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().SetInterspersed(false)
	commonVars(cmd.Flags(), &c.flags)

	return cmd
}

func maybeNameAndAction(cmd *cobra.Command, av []string) error {
	if cmd.ArgsLenAtDash() == 0 {
		// Workshop name is implicit if -- precedes all positional arguments
		return cobra.MinimumNArgs(1)(cmd, av)
	}

	argCount := len(av)
	if cmd.ArgsLenAtDash() < 0 && slices.Contains(av, "--") {
		argCount--
	}

	if argCount < 1 {
		return fmt.Errorf("requires at least 1 arg(s), only received %d", argCount)
	}
	return nil
}

func (c *CmdRun) Run(cmd *cobra.Command, av []string) error {
	args := &ExecArgs{action: true}

	// Infer workshop name if first positional argument is --
	if cmd.ArgsLenAtDash() == 0 {
		args.implicit = true
		args.command = av
	} else {
		// Remove first -- if cobra didn't see it
		if cmd.ArgsLenAtDash() < 0 {
			if i := slices.Index(av, "--"); i >= 0 {
				av = slices.Delete(slices.Clone(av), i, i+1)
			}
		}

		// Allow `workshop run action`. Passing arguments requires -- though.
		if len(av) <= 1 {
			args.implicit = true
			args.command = av
		} else {
			args.workshop = av[0]
			args.command = av[1:]
		}
	}

	return exec(c.root, &c.flags, args)
}

func (c *CmdRun) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	actionIdx := 1
	dashIdx := len(os.Args) - 2 - len(args)
	// TODO: Replace with cmd.ArgsLenAtDash() == 0.
	// See https://github.com/spf13/cobra/issues/1877.
	if dashIdx >= 0 && os.Args[dashIdx] == "--" {
		actionIdx = 0
	} else if actionIdx < len(args) && args[actionIdx] == "--" {
		actionIdx++
	}

	if actionIdx > len(args) {
		names, directive := c.root.doCompleteWorkshopNames(args, []string{"Ready", "Waiting"})

		// Try action names if no workshop names match partial argument.
		partialMatch := func(name string) bool {
			return strings.HasPrefix(name, toComplete)
		}
		if directive != cobra.ShellCompDirectiveError && !slices.ContainsFunc(names, partialMatch) {
			names, directive := completeActions(c.root, args)
			if directive != cobra.ShellCompDirectiveError {
				return names, directive
			}

		}

		return names, directive
	}
	if actionIdx < len(args) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return completeActions(c.root, args)
}

func completeActions(root *CmdRoot, args []string) ([]string, cobra.ShellCompDirective) {
	actionsCmd := CmdActions{root: root}
	actions, err := actionsCmd.actions(args)
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	slices.Sort(names)

	return names, cobra.ShellCompDirectiveNoFileComp
}

func commonVars(f *pflag.FlagSet, flags *ExecFlags) {
	f.StringVarP(&flags.WorkingDir, "cwd", "w", "/project", "Set the working directory in the workshop.")
	f.StringArrayVar(&flags.Env, "env", []string{}, "Set an environment variable, e.g. 'FOO=bar'; if only the name is provided, the value is inherited from the CLI environment.")
	f.IntVar(&flags.UserId, "uid", workshop.Uid, "Run as a specific workshop user.")
	f.IntVar(&flags.GroupId, "gid", workshop.Gid, "Run as a member of a specific workshop group.")
	f.DurationVar(&flags.Timeout, "timeout", 0, "Set a timeout; valid units are ns, us or µs, ms, s, m, h.")
	f.BoolVarP(&flags.Interactive, "interactive", "i", false, "Force interactive mode.")
	f.BoolVarP(&flags.NonInteractive, "non-interactive", "I", false, "Force non-interactive mode.")
}

func exec(root *CmdRoot, flags *ExecFlags, args *ExecArgs) error {
	if flags.Interactive && flags.NonInteractive {
		return errors.New("'-i' incompatible with '-I'")
	}

	cli, err := root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(root.project())
	if err != nil {
		return err
	}

	workshop := args.workshop
	if args.implicit {
		workshop, err = cli.SingleWorkshopName(project)
		if err != nil {
			return err
		}
	}

	if args.action {
		logger.Debugf("Running action %q", args.command)
	} else {
		// Obtain an exec session as close to a real login shell as possible.
		// This is required as an lxd exec call is a simple namespace exec, and LXD
		// ignores the instance configuration by design:
		// https://documentation.ubuntu.com/lxd/en/latest/instance-exec/#user-groups-and-working-directory

		command := []string{
			"sudo",
			"-u",
			"#" + strconv.Itoa(flags.UserId),
			"-g",
			"#" + strconv.Itoa(flags.GroupId),
			"--preserve-env",
			"--",
			"bash",
			"-l",
			"-c",
			`exec -- "$0" "$@"`,
		}
		args.command = append(command, args.command...)
		logger.Debugf("Running %q", args.command)
	}

	// Set up environment variables.
	env := make(map[string]string)

	term, ok := os.LookupEnv("TERM")
	if ok {
		env["TERM"] = term
	}

	// The runtime directory will most likely only be valid for the 'workshop'
	// user. This is created due to enabling lingering for this user during the
	// workshop start script and will not exist for other users. See:
	// https://github.com/systemd/systemd/blob/7419291670dd4066594350cce585031f60bc4f0a/src/login/pam_systemd.c#L1288
	xdgRuntimeDir := filepath.Join(dirs.XdgRuntimeDirBase, strconv.Itoa(flags.UserId))
	env["XDG_RUNTIME_DIR"] = xdgRuntimeDir

	// The session bus address is often determined programatically, however some
	// programs rely on an explicit environment variable. We set that here. Like
	// the runtime dir above, we only guarantee that the bus exists for the
	// workshop user.
	env["DBUS_SESSION_BUS_ADDRESS"] = "unix:path=" + filepath.Join(xdgRuntimeDir, "bus")

	for _, kv := range flags.Env {
		parts := strings.SplitN(kv, "=", 2)
		key := parts[0]

		var value string
		if len(parts) == 2 {
			value = parts[1]
		} else {
			value, ok = os.LookupEnv(key)
			if !ok {
				continue
			}
		}

		env[key] = value
	}

	stdoutIsTerminal := ptyutil.IsTerminal(unix.Stdout)

	// Specify Interactive=true if -i is given, or if stdin and stdout are TTYs.
	stdinIsTerminal := ptyutil.IsTerminal(unix.Stdin)
	var interactive bool
	if flags.Interactive {
		interactive = true
	} else if flags.NonInteractive {
		interactive = false
	} else {
		interactive = stdinIsTerminal && stdoutIsTerminal
	}

	// Record terminal state (and restore it before we exit).
	if interactive && stdinIsTerminal {
		oldState, err := ptyutil.MakeRaw(unix.Stdin)
		if err != nil {
			return fmt.Errorf("cannot switch terminal to raw mode: %v", err)
		}
		defer ptyutil.Restore(unix.Stdin, oldState)
	}

	// Grab current terminal dimensions.
	var width, height int
	if stdoutIsTerminal {
		var err error
		width, height, err = ptyutil.GetSize(unix.Stdout)
		if err != nil {
			return err
		}
	}

	// TODO: the lack of separate output in LXD exec when executing a command in
	// an interactive mode begets quirky things. Consider this: workshop exec
	// empty -- ls -R / 2>/dev/null Given that the command will be executed in
	// the interactive mode (stdin, stdout both point to the terminal), even if
	// ls produces access errors, those will not be filtered out to null as LXD
	// combines stderr and stdout in the interactive mode.
	opts := &client.ExecOptions{
		Command:     args.command,
		Environment: env,
		Action:      args.action,
		WorkingDir:  flags.WorkingDir,
		UserId:      &flags.UserId,
		GroupId:     &flags.GroupId,
		Interactive: interactive,
		Timeout:     flags.Timeout,
		Width:       width,
		Height:      height,
		Stdin:       Stdin,
		Stdout:      Stdout,
		Stderr:      Stderr,
	}

	// Start the command.
	process, err := cli.Exec(opts, workshop, project.Id)
	if err != nil {
		return err
	}

	// Start the control goroutine to handle signals and window resizing.
	stopControl := make(chan struct{})
	defer close(stopControl)
	sighup := make(chan struct{})
	go execControlHandler(process, interactive, stopControl, sighup)

	finished := make(chan error)
	go func() {
		finished <- process.Wait()
	}()

	// Wait for either the command to finish, or SIGHUP to be received.
	select {
	case err = <-finished:
		switch e := err.(type) {
		case nil:
			return nil
		case *client.ExitError:
			logger.Debugf("Process exited with code %d", e.ExitCode())
			return err
		default:
			return err
		}
	case <-sighup:
		// The \r is because we might be in raw mode, and it moves the cursor
		// back to the start of the line.
		fmt.Fprintf(os.Stderr, "SIGHUP received, exiting\r\n")
		// Exit with exit code 0 in this case (same behaviour as ssh).
		return nil
	}
}

func execControlHandler(process *client.ExecProcess, terminal bool, stop <-chan struct{}, sighup chan<- struct{}) {
	ch := make(chan os.Signal, 10)
	signal.Notify(ch,
		unix.SIGWINCH, unix.SIGHUP,
		unix.SIGTERM, unix.SIGINT, unix.SIGQUIT, unix.SIGABRT,
		unix.SIGTSTP, unix.SIGTTIN, unix.SIGTTOU, unix.SIGUSR1,
		unix.SIGUSR2, unix.SIGSEGV, unix.SIGCONT)

	for {
		var sig os.Signal
		select {
		case sig = <-ch:
		case <-stop:
			return
		}

		switch sig {
		case unix.SIGWINCH:
			if !terminal {
				logger.Debugf("Received 'SIGWINCH' signal in non-terminal mode, ignoring")
				break
			}
			logger.Debugf("Received '%s' signal, updating window geometry", sig)
			width, height, err := ptyutil.GetSize(unix.Stdout)
			if err != nil {
				logger.Debugf("Cannot get terminal size: %v", err)
				break
			}
			logger.Debugf("Window size is now: %dx%d", width, height)
			err = process.SendResize(width, height)
			if err != nil {
				logger.Debugf("Cannot set terminal size: %v", err)
				break //nolint:staticcheck // SA4011 Keep "ineffective" break for consistency
			}
		case unix.SIGHUP:
			logger.Debugf("Received 'SIGHUP' signal, forwarding and exiting")
			err := process.SendSignal(sig.(unix.Signal))
			if err != nil {
				logger.Debugf("Cannot forward signal '%s': %v", sig, err)
				break
			}
			close(sighup)
		case unix.SIGTERM, unix.SIGINT, unix.SIGQUIT, unix.SIGABRT,
			unix.SIGTSTP, unix.SIGTTIN, unix.SIGTTOU, unix.SIGUSR1,
			unix.SIGUSR2, unix.SIGSEGV, unix.SIGCONT:
			logger.Debugf("Received '%s' signal, forwarding to running process", sig)
			err := process.SendSignal(sig.(unix.Signal))
			if err != nil {
				logger.Debugf("Cannot forward signal '%s': %v", sig, err)
				break //nolint:staticcheck // SA4011 Keep "ineffective" break for consistency
			}
		}
	}
}

type CmdActions struct {
	root *CmdRoot
}

func (c *CmdActions) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions [<WORKSHOP>]",
		Args:  cobra.MaximumNArgs(1),
		Short: shortActionsHelp,
		Long:  longActionsHelp,
		Example: `
List actions for the 'nimble' workshop in the current project directory:
$ workshop actions nimble

The name is optional if the project has only one workshop:
$ workshop actions`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopName(nil),
	}

	return cmd
}

func (c *CmdActions) Run(cmd *cobra.Command, av []string) error {
	actions, err := c.actions(av)
	if err != nil || len(actions) == 0 {
		return err
	}

	encoder := yaml.NewEncoder(Stdout)
	encoder.SetIndent(2)
	return encoder.Encode(actions)
}

func (c *CmdActions) actions(av []string) (map[string]client.Action, error) {
	cli, err := c.root.client()
	if err != nil {
		return nil, err
	}

	p, err := cli.Project(c.root.project())
	if err != nil {
		return nil, err
	}

	if len(av) == 0 {
		name, err := cli.SingleWorkshopName(p)
		if err != nil {
			return nil, err
		}
		av = []string{name}
	}

	return cli.ListActions(p.Id, av[0])
}
