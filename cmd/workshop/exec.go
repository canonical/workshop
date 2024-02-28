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
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/canonical/workshop/client"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/ptyutil"
	"github.com/spf13/cobra"
)

type CmdExec struct {
	clientMixin
	WorkingDir     string        `short:"w"`
	Env            []string      `long:"env"`
	UserId         int           `long:"uid"`
	GroupId        int           `long:"gid"`
	Timeout        time.Duration `long:"timeout"`
	Interactive    bool          `short:"i"`
	NonInteractive bool          `short:"I"`
}

type CmdShellAlias struct {
	execCommand *CmdExec
}

var shortExecHelp = "Run a command and wait for it to complete."
var longExecHelp = `
The 'exec' subcommand runs an arbitrary command in the specified workshop,
waiting for it to complete. If a timeout elapses before that, it's terminated.

To accept an 'exec' command, the workshop must be *Ready* or *Pending*.
A command can run in two modes that determine how it handles standard streams:

- Interactively (for shell sessions)
- Non-interactively (for scripts)

To set the mode explicitly, use '-i' or '-I'. If neither is supplied,
'exec' deduces the mode based on the nature of its own streams:

- If stdin and stdout are terminals, the mode is interactive
- Otherwise, it's non-interactive

To separate the 'exec' subcommand from the command itself,
use shell syntax such as '--':

  $ workshop exec nimble -- echo -n foo bar

Notes:
- To start a workshop before running commands in it, use 'workshop start'
- You can set the working directory, environment variables, user and group ID
  for running the command in the workshop; reasonable defaults are provided
`

var shortShellHelp = "Start an interactive terminal session for the workshop."

func (c *CmdExec) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "exec <WORKSHOP>",
		Args:  cobra.MinimumNArgs(1),
		Short: shortExecHelp,
		Long:  longExecHelp,
		RunE:  c.Run,
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().StringVarP(&c.WorkingDir, "cwd", "w", "/project", "Set the working directory in the workshop")
	cmd.Flags().StringArrayVar(&c.Env, "env", []string{}, "Set an environment variable, e.g. 'FOO=bar'")
	cmd.Flags().IntVar(&c.UserId, "uid", 1000, "Run as a specific workshop user")
	cmd.Flags().IntVar(&c.GroupId, "gid", 1000, "Run as a member of a specific workshop group")
	cmd.Flags().DurationVar(&c.Timeout, "timeout", 0, "Set a timeout; valid units are 'ns', 'us'/'µs', 'ms', 's', 'm', 'h'")
	cmd.Flags().BoolVarP(&c.Interactive, "interactive", "i", false, "Force interactive mode")
	cmd.Flags().BoolVarP(&c.NonInteractive, "non-interactive", "I", false, "Force non-interactive mode")

	return cmd
}

func (c *CmdShellAlias) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "shell <WORKSHOP>",
		Args:  cobra.ExactArgs(1),
		Short: shortShellHelp,
		RunE:  c.Run,
	}

	c.execCommand = &CmdExec{}
	c.execCommand.UserId = 0
	c.execCommand.GroupId = 0
	return cmd
}

func (cmd *CmdShellAlias) Run(c *cobra.Command, av []string) error {
	return cmd.execCommand.Run(c, []string{av[0], "su", "-l", "workshop"})
}

func (cmd *CmdExec) Run(c *cobra.Command, av []string) error {
	if cmd.Interactive && cmd.NonInteractive {
		return errors.New("'-i' incompatible with '-I'")
	}

	cli, err := client.New(&ClientConfig)
	if err != nil {
		return fmt.Errorf("cannot create client: %v", err)
	}

	cmd.setClient(cli)

	project, err := cmd.client.Project(Project)
	if err != nil {
		return err
	}

	command := av[1:]
	logger.Debugf("Running %q", command)

	// Set up environment variables.
	env := make(map[string]string)
	term, ok := os.LookupEnv("TERM")
	if ok {
		env["TERM"] = term
	}

	for _, kv := range cmd.Env {
		parts := strings.SplitN(kv, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}
		env[key] = value
	}

	stdoutIsTerminal := ptyutil.IsTerminal(unix.Stdout)

	// Specify Interactive=true if -i is given, or if stdin and stdout are TTYs.
	stdinIsTerminal := ptyutil.IsTerminal(unix.Stdin)
	var interactive bool
	if cmd.Interactive {
		interactive = true
	} else if cmd.NonInteractive {
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
		Command:     command,
		Environment: env,
		WorkingDir:  cmd.WorkingDir,
		UserId:      &cmd.UserId,
		GroupId:     &cmd.GroupId,
		Interactive: interactive,
		Timeout:     cmd.Timeout,
		Width:       width,
		Height:      height,
		Stdin:       Stdin,
		Stdout:      Stdout,
		Stderr:      Stderr,
	}

	// Start the command.
	process, err := cmd.client.Exec(opts, av[0], project.Id)
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
