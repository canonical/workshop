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
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/ptyutil"
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
	script   bool
}

var shortExecHelp = "Run a command and wait for it to complete"
var longExecHelp = `
The 'exec' subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept an 'exec' command, the workshop must be 'Ready' or 'Pending'.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'exec' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'exec' subcommand from the command itself,
use shell syntax such as *--*:

  $ workshop exec nimble -- echo -n foo bar

This syntax is required if the workshop name is omitted.

Notes:

- To start a workshop before running commands in it, use 'workshop start'.

- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided.
`

var shortShellHelp = "Start an interactive terminal session for the workshop"
var longShellHelp = `
The 'shell' subcommand runs an interactive terminal session
in the specified workshop.

To accept a 'shell' command, the workshop must be 'Ready' or 'Pending'.


Notes:

- To start a workshop before running a terminal session, use 'workshop start'.

- The subcommand is a shorthand for 'workshop exec';
  it launches the login shell for 'workshop',
  the default non-privileged user in a workshop.
`

var shortRunHelp = "Run a workshop script and wait for it to complete"
var longRunHelp = `
The 'run' subcommand runs a script specified in the workshop definition file,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept a 'run' command, the workshop must be 'Ready' or 'Pending'.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)

- Non-interactively (for scripts)


To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'run' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive

- Otherwise, it's non-interactive


To separate the 'run' subcommand from the script and its arguments,
use shell syntax such as *--*:

$ workshop run nimble -- test --verbose

This syntax is required if the workshop name is omitted
and the script takes one or more arguments.

Notes:

- To start a workshop before running scripts in it, use 'workshop start'.

- You can set the working directory, environment variables, user and group ID
  for running the script in the workshop; reasonable defaults are provided.
`

var shortScriptsHelp = "List workshop scripts"
var longScriptsHelp = `
This command enumerates all scripts in the workshop, printing a YAML map.
`

func (c *CmdExec) Command() *cobra.Command {
	var cmd = &cobra.Command{
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
		RunE: c.Run,
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
	var cmd = &cobra.Command{
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
		RunE: c.Run,
	}

	return cmd
}

func (c *CmdShell) Run(cmd *cobra.Command, av []string) error {
	args := &ExecArgs{command: []string{"sudo", "-i", "-u", "workshop", "bash", "-c", "cd /project; exec bash"}}

	if len(av) > 0 {
		args.workshop = av[0]
	} else {
		args.implicit = true
	}

	return exec(c.root, &ExecFlags{WorkingDir: "/project"}, args)
}

func (c *CmdRun) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "run [flags] [<WORKSHOP>] [--] <SCRIPT> <ARGUMENTS>...",
		Args:  maybeNameAndScript,
		Short: shortRunHelp,
		Long:  longRunHelp,
		Example: `
Run the 'build' script under the 'nimble' workshop
in the current project directory:
$ workshop run nimble build

A similar command that sets an environment variable and the working directory:
$ workshop run --env GO111MODULE=off -w /project nimble build

The workshop name is optional if the project only has one workshop:
$ workshop run build

Scripts can accept arguments,
if a separator or a workshop name is provided:
$ workshop run -- build --debug
`,
		RunE: c.Run,
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().SetInterspersed(false)
	commonVars(cmd.Flags(), &c.flags)

	return cmd
}

func maybeNameAndScript(cmd *cobra.Command, av []string) error {
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
	args := &ExecArgs{script: true}

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

		// Allow `workshop run script`. Passing arguments requires -- though.
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

func commonVars(f *pflag.FlagSet, flags *ExecFlags) {
	f.StringVarP(&flags.WorkingDir, "cwd", "w", "/project", "Set the working directory in the workshop.")
	f.StringArrayVar(&flags.Env, "env", []string{}, "Set an environment variable, e.g. 'FOO=bar'; if only the name is provided, the value is inherited from the CLI environment.")
	f.IntVar(&flags.UserId, "uid", 1000, "Run as a specific workshop user.")
	f.IntVar(&flags.GroupId, "gid", 1000, "Run as a member of a specific workshop group.")
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

	project, err := cli.Project(root.project)
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

	if args.script {
		logger.Debugf("Running script %q", args.command)
	} else {
		logger.Debugf("Running %q", args.command)
	}

	// Set up environment variables.
	env := make(map[string]string)
	term, ok := os.LookupEnv("TERM")
	if ok {
		env["TERM"] = term
	}

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
		Script:      args.script,
		Environment: env,
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
				break
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
				break
			}
		}
	}
}

type CmdScripts struct {
	root *CmdRoot
}

func (c *CmdScripts) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "scripts",
		Args:  cobra.MaximumNArgs(1),
		Short: shortScriptsHelp,
		Long:  longScriptsHelp,
		Example: `
List scripts for the 'nimble' workshop in the current project directory:
$ workshop scripts nimble

The name is optional if the project has only one workshop:
$ workshop scripts`,
		RunE: c.Run,
	}

	return cmd
}

func (c *CmdScripts) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	p, err := cli.Project(c.root.project)
	if err != nil {
		return err
	}

	if len(av) == 0 {
		name, err := cli.SingleWorkshopName(p)
		if err != nil {
			return err
		}
		av = []string{name}
	}

	scripts, err := cli.ListScripts(p.Id, av[0])
	if err != nil || len(scripts) == 0 {
		return err
	}

	encoder := yaml.NewEncoder(Stdout)
	encoder.SetIndent(2)
	return encoder.Encode(scripts)
}
