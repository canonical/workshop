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

// Command try spawns an isolated subshell pre-configured to use a
// freshly built copy of the workshop binaries. Each invocation creates
// a session directory under ./try_sessions/, builds ./cmd/... into
// <session>/bin, starts workshopd against <session>, and drops the
// caller into a subshell with WORKSHOP_DATA, WORKSHOP_DEBUG and PATH
// set for the session. When the subshell exits, workshopd is terminated
// and the session directory is removed (unless --keep is given).
//
// When invoked from inside an existing try session (detected via
// WORKSHOP_DEV_TMP), the tool instead rebuilds the binaries into the
// same session directory, restarts workshopd, and returns without
// spawning a nested shell.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	envDevName        = "WORKSHOP_DEV_ENV"
	envDevTmp         = "WORKSHOP_DEV_TMP"
	envWorkshopData   = "WORKSHOP_DATA"
	envWorkshopCache  = "WORKSHOP_CACHE"
	envWorkshopDebug  = "WORKSHOP_DEBUG"
	envWorkshopSocket = "WORKSHOP_SOCKET"
	logFileName       = "workshopd.log"
	pidFileName       = "workshopd.pid"
	sessionsDir       = "try_sessions"
)

// buildBinaries runs `go install ./cmd/...` with GOBIN set to bin so
// the freshly built binaries land in the session directory rather than
// the user's $GOBIN.
func buildBinaries(bin string) error {
	fmt.Fprintln(os.Stderr, "building ./cmd/...")
	cmd := exec.Command("go", "install", "./cmd/...")
	cmd.Env = append(os.Environ(), "GOBIN="+bin)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("building ./cmd/...: %w", err)
	}

	fmt.Fprintln(os.Stderr, "build complete")
	return nil
}

// detectReentry inspects WORKSHOP_DEV_TMP and returns the existing
// session directory and true when the caller appears to already be
// inside a try session whose directory still exists. On any
// inconsistency it returns false, printing a warning so the caller can
// fall through to a fresh session.
func detectReentry() (string, bool) {
	tmp, ok := os.LookupEnv(envDevTmp)
	if !ok {
		return "", false
	}
	info, err := os.Stat(tmp)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(
			os.Stderr,
			"warning: %s=%q is not usable;"+
				" starting a fresh session\n",
			envDevTmp,
			tmp,
		)
		return "", false
	}
	return tmp, true
}

// fail prints err to stderr and exits the process with status 1.
func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	keep := flag.Bool(
		"keep",
		false,
		"keep the session directory after the subshell exits",
	)
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fail(fmt.Errorf("locating module root: %w", err))
	}
	err = os.Chdir(root)
	if err != nil {
		fail(fmt.Errorf(
			"changing to module root %q: %w", root, err,
		))
	}

	tmp, reentry := detectReentry()
	if reentry {
		err = restartSession(tmp)
		if err != nil {
			fail(fmt.Errorf("restarting session: %w", err))
		}
		return
	}

	err = runFreshSession(*keep)
	if err != nil {
		fail(fmt.Errorf("starting fresh session: %w", err))
	}
}

// moduleRoot returns the absolute path of the directory containing the
// enclosing module's go.mod, as reported by `go env GOMOD`.
func moduleRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("running `go env GOMOD`: %w", err)
	}

	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", errors.New("not inside a Go module")
	}

	return filepath.Dir(gomod), nil
}

// printBanner emits a short message identifying the active session and
// pointers to the workshopd log and re-entry behaviour.
func printBanner(name, tmp, shell string) {
	logPath := filepath.Join(tmp, logFileName)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "workshop try session: %s\n", name)
	fmt.Fprintf(os.Stderr, "  directory : %s\n", tmp)
	fmt.Fprintf(os.Stderr, "  workshopd : %s\n", logPath)
	fmt.Fprintf(os.Stderr, "  socket    : %s\n", socketPath(tmp))
	fmt.Fprintf(os.Stderr, "  shell     : %s\n", shell)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(
		os.Stderr,
		"Exit with Ctrl+D or `exit` to tear down the session.",
	)
	fmt.Fprintln(
		os.Stderr,
		"Re-run `go tool try` from inside this shell to"+
			" rebuild and restart workshopd.",
	)
	fmt.Fprintln(os.Stderr)
}

// restartSession rebuilds the binaries into tmp/bin, stops the current
// workshopd, and starts a fresh one against the same session. It does
// not spawn a new shell; control returns to the caller's existing one.
func restartSession(tmp string) error {
	bin := filepath.Join(tmp, "bin")
	err := buildBinaries(bin)
	if err != nil {
		return err
	}
	err = stopWorkshopd(tmp)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"warning: stopping previous workshopd: %v\n",
			err,
		)
	}
	// The new workshopd outlives this Go process; the original
	// session will tear it down via the updated PID file when the
	// user exits the shell.
	err = startWorkshopd(tmp)
	if err != nil {
		return err
	}
	fmt.Fprintln(
		os.Stderr,
		"rebuilt binaries and restarted workshopd in current"+
			" session",
	)
	return nil
}

// runFreshSession creates a new session directory, builds binaries
// into it, starts workshopd and spawns a configured subshell. Once the
// subshell exits, workshopd is stopped and the session directory is
// removed unless keep is true. When workshopd itself fails to start,
// the session directory is preserved so its log can be inspected.
func runFreshSession(keep bool) error {
	err := os.MkdirAll(sessionsDir, 0o755)
	if err != nil {
		return fmt.Errorf(
			"creating %s directory: %w", sessionsDir, err,
		)
	}

	rel, err := os.MkdirTemp(sessionsDir, "session-*")
	if err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	tmp, err := filepath.Abs(rel)
	if err != nil {
		return fmt.Errorf("resolving session path: %w", err)
	}

	bin := filepath.Join(tmp, "bin")
	err = os.MkdirAll(bin, 0o755)
	if err != nil {
		return fmt.Errorf("creating bin directory %q: %w", bin, err)
	}

	err = buildBinaries(bin)
	if err != nil {
		os.RemoveAll(tmp)
		return err
	}

	err = startWorkshopd(tmp)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"workshopd did not start;"+
				" session kept at %s for inspection\n",
			tmp,
		)
		return err
	}

	name := filepath.Base(tmp)
	shell := userShell()
	printBanner(name, tmp, shell)
	shellErr := runShell(shell, tmp, bin, name)

	stopErr := stopWorkshopd(tmp)
	if stopErr != nil {
		fmt.Fprintf(
			os.Stderr,
			"warning: stopping workshopd: %v\n",
			stopErr,
		)
	}

	sock := socketPath(tmp)
	rmSockErr := os.Remove(sock)
	if rmSockErr != nil && !errors.Is(rmSockErr, os.ErrNotExist) {
		fmt.Fprintf(
			os.Stderr,
			"warning: removing socket %s: %v\n",
			sock,
			rmSockErr,
		)
	}

	if keep {
		fmt.Fprintf(os.Stderr, "session kept at %s\n", tmp)
		return shellErr
	}

	err = os.RemoveAll(tmp)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"warning: removing %s: %v\n",
			tmp,
			err,
		)
	}

	return shellErr
}

// runShell launches the chosen shell with WORKSHOP_DATA, WORKSHOP_DEBUG,
// PATH, WORKSHOP_DEV_ENV and WORKSHOP_DEV_TMP configured for the
// session, and waits for it to exit. A non-zero exit from the shell
// itself is reported but not propagated as an error from runShell.
func runShell(shell, tmp, bin, name string) error {
	env := append(
		os.Environ(),
		envWorkshopData+"="+tmp,
		envWorkshopCache+"="+filepath.Join(tmp, "cache"),
		envWorkshopSocket+"="+socketPath(tmp),
		envWorkshopDebug+"=1",
		envDevName+"="+name,
		envDevTmp+"="+tmp,
		"PATH="+bin+":"+os.Getenv("PATH"),
	)

	cmd := exec.Command(shell)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = tmp

	err := cmd.Run()
	if err == nil {
		return nil
	}

	// A non-zero exit from the shell itself (e.g. the last command
	// failed) is the user's business, not a try failure.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}

	return fmt.Errorf("running shell %s: %w", shell, err)
}

// socketPath returns the path used for workshopd's Unix socket. It
// lives under os.TempDir() rather than inside the session directory so
// that long worktree paths do not exceed AF_UNIX's ~107-character
// limit on Linux.
func socketPath(tmp string) string {
	return filepath.Join(
		os.TempDir(),
		"workshop-"+filepath.Base(tmp)+".sock",
	)
}

// startWorkshopd starts workshopd as a background child of the current
// process. Output is redirected to workshopd.log inside tmp by a
// /bin/sh wrapper using `exec`, so the shell opens the file, dup2s
// onto its own fds, and then replaces itself with workshopd in place,
// preserving cmd.Process.Pid as workshopd's PID. The child runs with
// WORKSHOP_DATA, WORKSHOP_CACHE, WORKSHOP_SOCKET and WORKSHOP_DEBUG set
// so all of its state lives within or alongside the session directory.
// The PID is recorded in workshopd.pid; callers use stopWorkshopd to
// terminate the daemon.
func startWorkshopd(tmp string) error {
	logPath := filepath.Join(tmp, logFileName)
	// Pre-create the log file so any path or permission problems
	// surface here with a clear Go error rather than buried in a
	// shell redirection failure.
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf(
			"creating workshopd log %q: %w", logPath, err,
		)
	}
	logFile.Close()

	sock := socketPath(tmp)
	// A leftover socket file from a previous run prevents bind.
	err = os.Remove(sock)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"removing stale socket %q: %w", sock, err,
		)
	}

	binary := filepath.Join(tmp, "bin", "workshopd")
	// Paths come from filepath.Join inside the session directory and
	// are not expected to contain shell metacharacters; single quotes
	// keep the shell from interpreting anything in them.
	shellCmd := fmt.Sprintf(
		"exec '%s' run --create-dirs > '%s' 2>&1",
		binary,
		logPath,
	)
	cmd := exec.Command("/bin/sh", "-c", shellCmd)
	cmd.Env = append(
		os.Environ(),
		envWorkshopData+"="+tmp,
		envWorkshopCache+"="+filepath.Join(tmp, "cache"),
		envWorkshopSocket+"="+sock,
		envWorkshopDebug+"=1",
	)

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("starting workshopd: %w", err)
	}

	pidPath := filepath.Join(tmp, pidFileName)
	pidData := []byte(strconv.Itoa(cmd.Process.Pid))
	err = os.WriteFile(pidPath, pidData, 0o644)
	if err != nil {
		// Tear down the child since we have nowhere to record it;
		// best-effort, surface a warning if the signal itself fails.
		sigErr := cmd.Process.Signal(syscall.SIGTERM)
		if sigErr != nil {
			fmt.Fprintf(
				os.Stderr,
				"warning: signalling abandoned workshopd: %v\n",
				sigErr,
			)
		}
		return fmt.Errorf(
			"writing PID file %q: %w", pidPath, err,
		)
	}
	return nil
}

// stopWorkshopd reads the PID file in tmp and sends SIGTERM. We sleep
// briefly to give workshopd time to flush before the caller removes
// the session directory, but we do not poll for the PID to disappear:
// because the child is never wait()ed it lingers as a zombie until our
// parent process exits, at which point the kernel reaps it.
func stopWorkshopd(tmp string) error {
	pidPath := filepath.Join(tmp, pidFileName)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("reading PID file %q: %w", pidPath, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf(
			"parsing PID file %q: %w", pidPath, err,
		)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf(
			"locating workshopd PID %d: %w", pid, err,
		)
	}

	err = proc.Signal(syscall.SIGTERM)
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf(
			"signalling workshopd PID %d: %w", pid, err,
		)
	}

	time.Sleep(200 * time.Millisecond)
	return nil
}

// userShell returns the path of the user's preferred shell from
// $SHELL, falling back to /bin/bash.
func userShell() string {
	shell, ok := os.LookupEnv("SHELL")
	if ok {
		return shell
	}

	bash, err := exec.LookPath("bash")
	if err == nil {
		return bash
	}

	return "/bin/sh"
}
