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
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/canonical/x-go/strutil/shlex"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/cmd/internal/cmdutil"
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
	av            []string
	argsLenAtDash int
	action        bool
}

var shortExecHelp = "Run an ad-hoc shell command in a workshop"
var longExecHelp = `
The "exec" subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

Use "exec" for one-off shell commands typed on the command line.
To invoke a named script defined in the workshop's "actions:" section instead,
use "workshop run".

To accept an "exec" command, the workshop must be "Ready" or "Waiting".
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use "-i" or "-I". If neither is supplied,
"exec" deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the "exec" subcommand from the command itself,
use a separator (*--*).

If you omit the separator,
"exec" treats its first argument as the workshop name.
If the project has no such workshop
and the shell is interactive,
the argument is treated as a command to run in the default workshop.

Notes:

- To start a workshop before running commands in it, use "workshop start".

- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided.
`

var shortShellHelp = "Start an interactive terminal session for the workshop"
var longShellHelp = `
The "shell" subcommand runs an interactive terminal session
in the specified workshop.

To accept a "shell" command, the workshop must be "Ready" or "Waiting".


Notes:

- To start a workshop before running a terminal session, use "workshop start".

- The subcommand is a shorthand for "workshop exec";
  it launches the login shell for "workshop",
  the default non-privileged user in a workshop.
`

var shortRunHelp = "Run a named action from the workshop definition"
var longRunHelp = `
The "run" subcommand runs an action specified in the workshop definition file,
waiting for it to complete. If a timeout elapses before that, it's terminated.

Use "run" to invoke a named action defined in the workshop's "actions:" section.
To list available actions, use "workshop actions".
To run an ad-hoc shell command instead, use "workshop exec".

To accept a "run" command, the workshop must be "Ready" or "Waiting".
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use "-i" or "-I". If neither is supplied,
"run" deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the "run" subcommand from the action and its arguments,
use a separator (*--*).

If you omit the separator,
"run" treats its first argument as the workshop name.
If the project has no such workshop
and the shell is interactive,
the argument is treated as an action to run in the default workshop.

Any trailing arguments are forwarded to the action as positional parameters,
so action scripts can consume them with standard shell expansions.

Notes:

- To start a workshop before running actions in it, use "workshop start".

- You can set the working directory, environment variables, user and group ID
  for running the action in the workshop; reasonable defaults are provided.
`

var shortActionsHelp = "List the named actions defined in a workshop"
var longActionsHelp = `
This command enumerates all actions in the workshop, printing a YAML map.
`

func (c *CmdExec) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "exec [flags] [<WORKSHOP>] [--] <COMMAND>...",
		Args:    cobra.MinimumNArgs(1),
		Short:   shortExecHelp,
		GroupID: GrpExec,
		Long:    longExecHelp,
		Example: `
Run the "go build main.go" command under the "nimble" workshop
in the current project directory:
$ workshop exec nimble -- go build main.go

A similar command that sets an environment variable and the working directory:
$ workshop exec --env GO111MODULE=off -w /project nimble -- go build -x

Run a command as root (the default is "workshop"):
$ workshop exec --uid 0 nimble id

Run a custom interactive shell:
$ workshop exec -I nimble sh

If the project has only one workshop, the workshop name is optional:
$ workshop exec -- sh

If the command doesn't overlap with a workshop name
and the shell is interactive,
the separator is also optional:
$ workshop exec sh`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().SetInterspersed(false)
	commonVars(cmd.Flags(), &c.flags)
	_ = cmd.RegisterFlagCompletionFunc("env", cmdutil.CompleteEnv)

	return cmd
}

func (c *CmdExec) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}

	return c.root.doCompleteWorkshopNames(args, []string{"Ready", "Waiting"})
}

func (c *CmdExec) Run(cmd *cobra.Command, av []string) error {
	args := execArgs(av, cmd.ArgsLenAtDash())
	return exec(c.root, &c.flags, args)
}

func execArgs(av []string, argsLenAtDash int) *ExecArgs {
	args := &ExecArgs{av: av, argsLenAtDash: argsLenAtDash}
	// With SetInterspersed(false), cobra ignores all but the first
	// positional argument, so we need to remove `--` manually. We leave
	// it in if it's part of the command. To summarise:
	// - workshop exec -- sh: cmd.ArgsLenAtDash() == 0, av == [sh]
	// - workshop exec ws -- sh: cmd.ArgsLenAtDash() < 0, av == [ws -- sh]
	if args.argsLenAtDash < 0 && 1 < len(av) && av[1] == "--" {
		args.av = slices.Delete(slices.Clone(av), 1, 2)
		args.argsLenAtDash = 1
	}
	return args
}

func (c *CmdShell) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "shell [<WORKSHOP>]",
		Args:    cobra.MaximumNArgs(1),
		Short:   shortShellHelp,
		GroupID: GrpExec,
		Long:    longShellHelp,
		Example: `
Open the default login shell of the "workshop" user into the "nimble" workshop
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
	flags := &ExecFlags{
		WorkingDir: "/project",
		UserId:     workshop.Uid,
		GroupId:    workshop.Gid,
	}
	// With no arguments, we call `exec -- bash`. With one argument, we
	// call `exec av[0] -- bash`.
	args := &ExecArgs{
		av:            append(slices.Clone(av), "bash"),
		argsLenAtDash: len(av),
	}
	return exec(c.root, flags, args)
}

func (c *CmdRun) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "run [flags] [<WORKSHOP>] [--] <ACTION> <ARGUMENTS>...",
		Args:    cobra.MinimumNArgs(1),
		Short:   shortRunHelp,
		GroupID: GrpActions,
		Long:    longRunHelp,
		Example: `
Run the "build" action under the "nimble" workshop
in the current project directory:
$ workshop run nimble build

A similar command that sets an environment variable and the working directory:
$ workshop run --env GO111MODULE=off -w /project nimble -- build

If the project has only one workshop, the workshop name is optional:
$ workshop run -- build

If the action doesn't overlap with a workshop name
and the shell is interactive,
the separator is also optional:
$ workshop run build

Forward arguments to an action that consumes them
(for example, ` + "``tests: go test \"$@\"``" + ` in the workshop definition):
$ workshop run dev -- tests -run TestFoo ./pkg/...`,
		RunE:              c.Run,
		ValidArgsFunction: c.complete,
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().SetInterspersed(false)
	commonVars(cmd.Flags(), &c.flags)
	_ = cmd.RegisterFlagCompletionFunc("env", cmdutil.CompleteEnv)

	return cmd
}

func (c *CmdRun) Run(cmd *cobra.Command, av []string) error {
	args := execArgs(av, cmd.ArgsLenAtDash())
	args.action = true
	return exec(c.root, &c.flags, args)
}

func (c *CmdRun) complete(cmd *cobra.Command, av []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// TODO: Replace argsLenAtDash with cmd.ArgsLenAtDash().
	// See https://github.com/spf13/cobra/issues/1877.
	argsLenAtDash := -1
	argsIdx := len(os.Args) - 1 - len(av)
	if argsIdx > 0 && os.Args[argsIdx-1] == "--" {
		argsLenAtDash = 0
	}
	args := execArgs(av, argsLenAtDash)

	// We only complete workshop names and actions. Anything beyond that
	// must be an argument to the action. The case len(args.av) == 1 and
	// args.argsLenAtDash < 0 is ambiguous, we check later on.
	if len(args.av) > 1 || (args.argsLenAtDash == 0 && len(args.av) > 0) {
		return nil, cobra.ShellCompDirectiveDefault
	}

	cli, err := c.root.noRetryClient()
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	project, err := cli.Project(c.root.project())
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Argument after a separator must be an action.
	if args.argsLenAtDash >= 0 {
		return completeActions(cli, project, args.av)
	}

	workshop, err := cli.SingleWorkshopName(project)
	if len(args.av) >= 1 {
		// If there's a single workshop and the first argument isn't
		// the workshop name, it must be an action and we don't know
		// how to complete the second argument.
		if err == nil && args.av[0] != workshop {
			return nil, cobra.ShellCompDirectiveDefault
		}
		// Otherwise the second argument is an action.
		return completeActions(cli, project, args.av)
	}
	// If there are multiple workshops, first argument must be a workshop name.
	if err != nil {
		return completeWorkshopNames(cli, project, args.av, []string{"Ready", "Waiting"})
	}

	// With a single workshop, complete actions first.
	actions, directive := completeActions(cli, project, []string{workshop})
	// If no actions match, but the workshop name does, complete it instead.
	partialMatch := func(name string) bool { return strings.HasPrefix(name, toComplete) }
	if !slices.ContainsFunc(actions, partialMatch) && strings.HasPrefix(workshop, toComplete) {
		return []string{workshop}, cobra.ShellCompDirectiveNoFileComp
	}
	return actions, directive
}

func completeActions(cli *client.Client, p *client.Project, args []string) ([]string, cobra.ShellCompDirective) {
	actions, err := listActions(cli, p, args)
	if err != nil {
		cobra.CompDebugln(err.Error(), false)
		return nil, cobra.ShellCompDirectiveNoFileComp
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
		return errors.New(`"-i" incompatible with "-I"`)
	}

	cli, err := root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(root.project())
	if err != nil {
		return err
	}

	// Specify Interactive=true if -i is given, or if stdin and stdout are TTYs.
	stdinIsTerminal := ptyutil.IsTerminal(unix.Stdin)
	stdoutIsTerminal := ptyutil.IsTerminal(unix.Stdout)
	var interactive bool
	if flags.Interactive {
		interactive = true
	} else if flags.NonInteractive {
		interactive = false
	} else {
		interactive = stdinIsTerminal && stdoutIsTerminal
	}

	workshop, av, err := splitExecArgs(cli, project, interactive, args)
	if err != nil {
		return err
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

	// The session bus address is often determined programmatically, however some
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

	var commandPrefix []string
	if args.action {
		logger.Debugf("Running action %q", av)
	} else {
		// Obtain an exec session as close to a real login shell as possible.
		// This is required as an lxd exec call is a simple namespace exec, and LXD
		// ignores the instance configuration by design:
		// https://canonical.com/lxd/docs/latest/instance-exec/#user-groups-and-working-directory
		commandPrefix = []string{
			"sudo",
			"-u",
			"#" + strconv.Itoa(flags.UserId),
			"-g",
			"#" + strconv.Itoa(flags.GroupId),
		}

		keys := slices.Sorted(maps.Keys(env))
		for _, k := range keys {
			commandPrefix = append(commandPrefix, "--preserve-env="+k)
		}

		commandPrefix = append(
			commandPrefix,
			"--",
			"bash",
			"-l",
			"-c",
			`exec -- "$0" "$@"`,
		)
		logger.Debugf("Running command %q", av)
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
		Command:       av,
		CommandPrefix: commandPrefix,
		Environment:   env,
		Action:        args.action,
		WorkingDir:    flags.WorkingDir,
		UserId:        &flags.UserId,
		GroupId:       &flags.GroupId,
		Interactive:   interactive,
		Timeout:       flags.Timeout,
		Width:         width,
		Height:        height,
		Stdin:         Stdin,
		Stdout:        Stdout,
		Stderr:        Stderr,
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

// Extract a workshop and the command from the arguments.
func splitExecArgs(cli *client.Client, project *client.Project, interactive bool, args *ExecArgs) (string, []string, error) {
	// The first argument is a workshop name if followed by a separator,
	// but there might not be enough arguments after that.
	if args.argsLenAtDash == 1 {
		return splitWorkshopAndCommand(args)
	}

	workshop, err := cli.SingleWorkshopName(project)
	// When a separator is the first argument, we always infer the name.
	if args.argsLenAtDash == 0 {
		return workshop, args.av, err
	}
	// If there's no separator and the first argument doesn't match the
	// inferred name, it can only be a command. Allowed in interactive mode
	// since the user likely intended this interpretation. In scripts,
	// the author may have named a workshop that no longer exists, so we
	// suggest a more robust approach for non-interactive sessions.
	if err == nil && workshop != args.av[0] {
		if interactive {
			return workshop, args.av, err
		}
		action, subject := actionSubject(args.action)
		suggest := shlex.Join([]string{"workshop", action, "--", args.av[0]})
		return "", nil, fmt.Errorf(`unclear if %q names a workshop or %s: try "%s"`, args.av[0], subject, suggest)
	}
	if err != nil {
		// Return the error unless there are multiple workshops and the
		// first argument names one of them.
		var singleErr *client.SingleWorkshopError
		if !errors.As(err, &singleErr) || !slices.Contains(singleErr.Names, args.av[0]) {
			return "", nil, err
		}
	}

	// The first argument is a workshop name, but there might not
	// be enough arguments following it.
	return splitWorkshopAndCommand(args)
}

func splitWorkshopAndCommand(args *ExecArgs) (string, []string, error) {
	if len(args.av) > 1 {
		return args.av[0], args.av[1:], nil
	}

	action, subject := actionSubject(args.action)
	return "", nil, fmt.Errorf("cannot %s %s in %q: must specify %s", action, subject, args.av[0], subject)
}

func actionSubject(action bool) (string, string) {
	if action {
		return "run", "action"
	}
	return "exec", "command"
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
				logger.Debugf(`Received "SIGWINCH" signal in non-terminal mode, ignoring`)
				break
			}
			logger.Debugf("Received %q signal, updating window geometry", sig)
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
			logger.Debugf(`Received "SIGHUP" signal, forwarding and exiting`)
			err := process.SendSignal(sig.(unix.Signal))
			if err != nil {
				logger.Debugf("Cannot forward signal %q: %v", sig, err)
				break
			}
			close(sighup)
		case unix.SIGTERM, unix.SIGINT, unix.SIGQUIT, unix.SIGABRT,
			unix.SIGTSTP, unix.SIGTTIN, unix.SIGTTOU, unix.SIGUSR1,
			unix.SIGUSR2, unix.SIGSEGV, unix.SIGCONT:
			logger.Debugf("Received %q signal, forwarding to running process", sig)
			err := process.SendSignal(sig.(unix.Signal))
			if err != nil {
				logger.Debugf("Cannot forward signal %q: %v", sig, err)
				break //nolint:staticcheck // SA4011 Keep "ineffective" break for consistency
			}
		}
	}
}

type CmdActions struct {
	root *CmdRoot
}

func (c *CmdActions) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "actions [<WORKSHOP>]",
		Args:    cobra.MaximumNArgs(1),
		Short:   shortActionsHelp,
		GroupID: GrpActions,
		Long:    longActionsHelp,
		Example: `
List actions for the "nimble" workshop in the current project directory:
$ workshop actions nimble

The name is optional if the project has only one workshop:
$ workshop actions`,
		RunE:              c.Run,
		ValidArgsFunction: c.root.completeWorkshopName(nil),
	}

	return cmd
}

func (c *CmdActions) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	p, err := cli.Project(c.root.project())
	if err != nil {
		return err
	}

	actions, err := listActions(cli, p, av)
	if err != nil || len(actions) == 0 {
		return err
	}

	encoder := yaml.NewEncoder(Stdout)
	encoder.SetIndent(2)
	return encoder.Encode(actions)
}

func listActions(cli *client.Client, p *client.Project, av []string) (map[string]client.Action, error) {
	if len(av) == 0 {
		name, err := cli.SingleWorkshopName(p)
		if err != nil {
			return nil, err
		}
		av = []string{name}
	}

	return cli.ListActions(p.Id, av[0])
}
