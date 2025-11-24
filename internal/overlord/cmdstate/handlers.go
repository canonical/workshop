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

package cmdstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/canonical/lxd/shared/api"
	"github.com/gorilla/websocket"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/logger"
	. "github.com/canonical/workshop/internal/overlord/handlersetup"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
	"github.com/canonical/workshop/internal/wsutil"
)

const (
	connectTimeout   = 5 * time.Second
	handshakeTimeout = 5 * time.Second

	wsControl = "control"
	wsStdio   = "stdio"
	wsStdout  = "stdout"
	wsStderr  = "stderr"
)

// execution tracks the execution of a command.
type execution struct {
	workshop string
	execArgs *workshop.ExecArgs

	websockets       map[string]*websocket.Conn
	websocketsLock   sync.Mutex
	ioConnected      chan struct{}
	controlConnected chan struct{}
}

func (m *CommandManager) doExec(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	st := task.State()
	st.Lock()
	argsObj := st.Cached(ExecArgsKey(task.ID()))
	st.Unlock()
	args, ok := argsObj.(*workshop.ExecArgs)
	if !ok || args == nil {
		return fmt.Errorf("cannot get exec args for task %q: task was probably interrupted", task.ID())
	}

	// Set up the object that will track the execution.
	e := &execution{
		workshop:         w,
		execArgs:         args,
		websockets:       make(map[string]*websocket.Conn),
		ioConnected:      make(chan struct{}),
		controlConnected: make(chan struct{}),
	}

	// Populate the websockets map (with nil connections until connected).
	e.websockets[wsControl] = nil
	e.websockets[wsStdio] = nil
	e.websockets[wsStdout] = nil
	e.websockets[wsStderr] = nil

	// Store the execution object on the manager (for Connect).
	m.executionsMutex.Lock()
	m.executions[task.ID()] = e
	m.executionsMutex.Unlock()
	m.executionsCond.Broadcast() // signal that Connects can start happening
	defer func() {
		m.executionsMutex.Lock()
		delete(m.executions, task.ID())
		m.executionsMutex.Unlock()
	}()

	// Run the command! Killing the tomb will terminate the command.
	return e.do(ctx, task, m.backend)
}

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin:      func(r *http.Request) bool { return true },
	HandshakeTimeout: handshakeTimeout,
}

func (e *execution) connect(r *http.Request, w http.ResponseWriter, id string) error {
	e.websocketsLock.Lock()
	conn, ok := e.websockets[id]
	e.websocketsLock.Unlock()
	if !ok {
		return os.ErrNotExist
	}
	if conn != nil {
		return fmt.Errorf("%s websocket already connected", id)
	}

	// Upgrade the HTTP connection to a websocket connection.
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	// Save the connection.
	e.websocketsLock.Lock()
	defer e.websocketsLock.Unlock()
	e.websockets[id] = conn

	// Signal that we're connected.
	if id == wsControl {
		close(e.controlConnected)
	} else if e.websockets[wsStdio] != nil &&
		(e.execArgs.Interactive || (e.websockets[wsStderr] != nil && e.websockets[wsStdout] != nil)) {
		// stio for everything in interactive and stdio/stdout/stderr separate otherwise
		close(e.ioConnected)
	}
	return nil
}

func (e *execution) getWebsocket(key string) *websocket.Conn {
	e.websocketsLock.Lock()
	defer e.websocketsLock.Unlock()
	return e.websockets[key]
}

// waitIOConnected waits till all the I/O websockets are connected or the
// connect timeout elapses (or the provided ctx is cancelled).
func (e *execution) waitIOConnected(ctx context.Context, execID string) error {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			logger.Noticef("Exec %s: timeout waiting for websocket connections: %v", execID, ctx.Err())
			return fmt.Errorf("exec %s: timeout waiting for websocket connections: %w", execID, ctx.Err())
		}
		return ctx.Err()
	case <-e.ioConnected:
		return nil
	}
}

// do actually runs the command.
func (e *execution) do(ctx context.Context, task *state.Task, backend workshop.Backend) error {
	// Wait till client has connected to "stdio" websocket (and "stderr" if
	// separating stderr), to avoid race conditions forwarding I/O.
	err := e.waitIOConnected(ctx, task.ID())
	if err != nil {
		return err
	}

	// Files/pipes to close before and after waiting for output to be finished sending.
	var beforeClosers []io.Closer
	var afterClosers []io.Closer

	stdinReader, stdinWriter := io.Pipe()
	afterClosers = append(afterClosers, stdinReader)

	stdoutReader, stdoutWriter := io.Pipe()
	beforeClosers = append(beforeClosers, stdoutWriter)
	afterClosers = append(afterClosers, stdoutReader)

	var stderrReader *io.PipeReader
	var stderrWriter *io.PipeWriter

	// Closed to make the controlLoop stop early.
	stopControl := make(chan struct{})
	defer close(stopControl)

	var wgOutputSent sync.WaitGroup

	ioConn := e.getWebsocket(wsStdio)
	// read stdin from the connection and redirect it to the exec's input
	go func() {
		logger.Debugf("Exec %s: started mirroring stdin websocket", task.ID())
		defer logger.Debugf("Exec %s: finished mirroring stdin websocket", task.ID())
		<-wsutil.WebsocketRecvStream(stdinWriter, ioConn)
		stdinWriter.Close()
	}()

	if e.execArgs.Interactive {
		wgOutputSent.Go(func() {
			logger.Debugf("Exec %s: started mirroring stdout websocket", task.ID())
			defer logger.Debugf("Exec %s: finished mirroring stdout websocket", task.ID())
			<-wsutil.WebsocketSendStream(ioConn, stdoutReader, -1)
		})
	} else {
		stdoutConn := e.getWebsocket(wsStdout)
		wgOutputSent.Go(func() {
			logger.Debugf("Exec %s: started mirroring stdout websocket", task.ID())
			defer logger.Debugf("Exec %s: finished mirroring stdout websocket", task.ID())
			<-wsutil.WebsocketSendStream(stdoutConn, stdoutReader, -1)
		})

		stderrConn := e.getWebsocket(wsStderr)
		stderrReader, stderrWriter = io.Pipe()
		wgOutputSent.Go(func() {
			logger.Debugf("Exec %s: started mirroring stderr websocket", task.ID())
			defer logger.Debugf("Exec %s: finished mirroring stderr websocket", task.ID())
			<-wsutil.WebsocketSendStream(stderrConn, stderrReader, -1)
		})
		beforeClosers = append(beforeClosers, stderrWriter)
		afterClosers = append(afterClosers, stderrReader)
	}

	if e.execArgs.Timeout != 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, e.execArgs.Timeout)
		defer cancel()
	}

	// TODO: the lack of separate output in LXD exec when executing a command in
	// an interactive mode begets quirky things. Consider this: workshop exec
	// empty -- ls -R / 2>/dev/null Given that the command will be executed in
	// the interactive mode (stdin, stdout both point to the terminal), even if
	// ls produces access errors, those will not be filtered out to null as LXD
	// combines stderr and stdout in the interactive mode.
	exectx, err := backend.Exec(ctx, e.workshop, &workshop.Execution{
		ExecArgs: *e.execArgs,
		ExecControls: workshop.ExecControls{
			Stdin:  stdinReader,
			Stdout: stdoutWriter,
			Stderr: stderrWriter,
			Control: func(conn *websocket.Conn) {
				e.controlLoop(task.ID(), conn, stopControl)
			},
		},
	})

	// if the command was initiated successfully, wait for the execution
	// otherwise, move directly to the clean up, report and exit
	if err == nil {
		err = exectx.WaitExecution(ctx)
	}

	// Close the control channel, if connected.
	controlConn := e.getWebsocket(wsControl)
	if controlConn != nil {
		_ = controlConn.Close()
	}

	// Close open files and channels.
	for _, closer := range beforeClosers {
		_ = closer.Close()
	}

	wgOutputSent.Wait()

	for _, closer := range afterClosers {
		_ = closer.Close()
	}

	// only set the error exit status in the task's metadata if the error
	// belongs to the command execution (e.g. not an LXD error)
	if err == nil {
		setExitCode(task, 0)
	} else {
		if execerr, ok := err.(*workshop.ErrExec); ok {
			setExitCode(task, execerr.Status)
			return nil
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("timed out after %v: %w", e.execArgs.Timeout, ctx.Err())
		}
	}

	return err
}

func setExitCode(task *state.Task, exitCode int) {
	st := task.State()
	st.Lock()
	defer st.Unlock()
	task.Set("api-data", map[string]interface{}{
		"exit-code": exitCode,
	})
}

func (e *execution) controlLoop(execId string, conn *websocket.Conn, stop <-chan struct{}) {
	logger.Debugf("Exec %s: control handler waiting", execId)
	defer logger.Debugf("Exec %s: control handler finished", execId)

	// Wait till the control websocket is connected.
	select {
	case <-e.controlConnected:
		break
	case <-stop:
		return
	}

	logger.Debugf("Exec %s: control handler started for %s", execId, e.execArgs.Command)
	for {
		controlConn := e.getWebsocket(wsControl)
		mt, r, err := controlConn.NextReader()
		if mt == websocket.CloseMessage {
			break
		}

		if err != nil {
			logger.Debugf("Exec %s: cannot get next websocket reader for %s: %v", execId, e.execArgs.Command, err)
			break
		}

		var command api.InstanceExecControl
		err = json.NewDecoder(r).Decode(&command)
		if err != nil {
			logger.Noticef("Exec %s: cannot decode control websocket command: %v", execId, err)
			continue
		}

		err = conn.WriteJSON(command)
		if err != nil {
			logger.Noticef("Exec %s: cannot send control websocket command: %v", execId, err)
		}
	}
}

func (m *CommandManager) doInstallAction(task *state.Task, tomb *tomb.Tomb) error {
	user, prj, w, err := UserProjectWorkshop(task)
	if err != nil {
		return err
	}

	ctx, cancel := BackendContext(tomb, user, prj.ProjectId)
	defer cancel()

	var execTask string
	st := task.State()
	st.Lock()
	err = task.Get("exec-task", &execTask)
	st.Unlock()
	if err != nil {
		return fmt.Errorf("cannot get exec task for task %q: %w", task.ID(), err)
	}
	st.Lock()
	argsObj := st.Cached(ExecArgsKey(task.ID()))
	st.Unlock()
	argsOld, ok := argsObj.(*workshop.ExecArgs)
	if !ok || argsOld == nil {
		return fmt.Errorf("cannot get exec args for task %q: task was probably interrupted", task.ID())
	}
	// Shallow copy to avoid modifying original object.
	args := *argsOld

	name := args.Command[0]
	file, err := prj.Workshop(w)
	if err != nil {
		return err
	}

	action, ok := file.Actions[name]
	if !ok {
		return errors.New("action not found")
	}

	path := filepath.Join(dirs.WorkshopActionsDir, name)
	command := []string{
		"sudo",
		"-u",
		"#" + strconv.Itoa(args.UserId),
		"-g", "#" + strconv.Itoa(args.GroupId),
		"--preserve-env",
		"--",
		"bash",
		"-elo",
		"pipefail",
		path,
	}
	args.Command = append(command, args.Command[1:]...)

	wfs, err := m.backend.WorkshopFs(ctx, w)
	if err != nil {
		return err
	}
	defer wfs.Close()

	if err := wfs.MkdirAll(dirs.WorkshopActionsDir, 0o755); err != nil {
		return err
	}

	if err := wfs.AtomicWriteTo(strings.NewReader(string(action)), path, 0o644); err != nil {
		return err
	}

	st.Lock()
	st.Cache(ExecArgsKey(execTask), &args)
	st.Unlock()

	return nil
}
