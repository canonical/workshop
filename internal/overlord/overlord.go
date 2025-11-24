// Copyright (c) 2014-2020 Canonical Ltd
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

// Package overlord is the central control base, and ruler of all things.
package overlord

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/canonical/x-go/randutil"
	"gopkg.in/tomb.v2"

	"github.com/canonical/workshop/internal/dirs"
	store "github.com/canonical/workshop/internal/fakestore"
	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/overlord/cmdstate"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/hookstate"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/patch"
	"github.com/canonical/workshop/internal/overlord/restart"
	"github.com/canonical/workshop/internal/overlord/sdkstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/overlord/workshopstate"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/systemd"
	"github.com/canonical/workshop/internal/workshop"
	lxdbackend "github.com/canonical/workshop/internal/workshop/lxd"
)

var (
	ensureInterval = 5 * time.Minute
	pruneInterval  = 10 * time.Minute
	pruneWait      = 24 * time.Hour * 1
	abortWait      = 24 * time.Hour * 7

	stateLockTimeout       = 1 * time.Minute
	stateLockRetryInterval = 1 * time.Second

	pruneMaxChanges = 500

	systemdSdNotify = systemd.SdNotify
)

var pruneTickerC = func(t *time.Ticker) <-chan time.Time {
	return t.C
}

var workshopBackendOverride workshop.Backend

// Overlord is the central manager of the system, keeping track
// of all available state managers and related helpers.
type Overlord struct {
	stateFLock *osutil.FileLock

	stateDir string
	stateEng *StateEngine

	// ensure loop
	loopTomb    *tomb.Tomb
	ensureLock  sync.Mutex
	ensureTimer *time.Timer
	ensureNext  time.Time
	ensureRun   int32
	pruneTicker *time.Ticker

	// managers
	inited      bool
	startedUp   bool
	sdkmgr      *sdkstate.SdkManager
	workshopmgr *workshopstate.WorkshopManager
	hookmgr     *hookstate.HookManager
	commandmgr  *cmdstate.CommandManager
	ifacemgr    *ifacestate.InterfaceManager
	runner      *state.TaskRunner

	startOfOperationTime time.Time
}

// New creates a new Overlord with all its state managers.
// It can be provided with an optional restart.Handler.
func New(dir string, restartHandler restart.Handler) (*Overlord, error) {
	o := &Overlord{
		stateDir: dir,
		loopTomb: new(tomb.Tomb),
		inited:   true,
	}

	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("directory %q must be absolute", dir)
	}
	if !osutil.IsDir(dir) {
		return nil, fmt.Errorf("directory %q does not exist", dir)
	}

	statePath := filepath.Join(dir, "state.json")

	backend := &overlordStateBackend{
		path:         statePath,
		ensureBefore: o.ensureBefore,
	}
	s, err := o.loadState(statePath, restartHandler, backend)
	if err != nil {
		return nil, err
	}

	o.stateEng = NewStateEngine(s)
	o.runner = state.NewTaskRunner(s)

	sto := store.New()
	sdk.ReplaceStore(s, sto)

	if workshopBackendOverride != nil {
		workshop.ReplaceBackend(s, workshopBackendOverride)
	} else {
		wbe, err := lxdbackend.New()
		if err != nil {
			return nil, err
		}
		workshop.ReplaceBackend(s, wbe)
	}

	// any unknown task should be ignored and succeed
	matchAnyUnknownTask := func(_ *state.Task) bool {
		return true
	}
	o.runner.AddOptionalHandler(matchAnyUnknownTask, handleUnknownTask, nil)

	o.workshopmgr = workshopstate.New(s, o.runner)
	o.addManager(o.workshopmgr)

	o.hookmgr = hookstate.New(s, o.runner)
	o.addManager(o.hookmgr)

	healthstate.Init(o.hookmgr)

	o.commandmgr = cmdstate.New(s, o.runner)
	o.addManager(o.commandmgr)

	o.ifacemgr = ifacestate.New(s, o.runner)
	o.addManager(o.ifacemgr)

	o.sdkmgr = sdkstate.New(s, o.runner, o.ifacemgr.Repository())
	o.addManager(o.sdkmgr)

	// the shared task runner should be added last!
	o.stateEng.AddManager(o.runner)

	return o, nil
}

func (se *Overlord) StartUp() error {
	if se.startedUp {
		return nil
	}
	se.startedUp = true

	var err error
	st := se.State()
	st.Lock()
	se.startOfOperationTime, err = se.StartOfOperationTime()
	st.Unlock()
	if err != nil {
		return fmt.Errorf("cannot get start of operation time: %w", err)
	}
	return se.stateEng.StartUp()
}

var timeNow = time.Now

// StartOfOperationTime returns the time when workshop started operating,
// and sets it in the state when called for the first time.
func (m *Overlord) StartOfOperationTime() (time.Time, error) {
	var opTime time.Time
	err := m.State().Get("start-of-operation-time", &opTime)
	if !errors.Is(err, state.ErrNoState) {
		return opTime, err
	}

	opTime = timeNow()
	m.State().Set("start-of-operation-time", opTime)
	return opTime, nil
}

func (o *Overlord) addManager(mgr StateManager) {
	o.stateEng.AddManager(mgr)
}

func initStateFileLock() (*osutil.FileLock, error) {
	lockFilePath := dirs.WorkshopStateLockFile
	if err := os.MkdirAll(filepath.Dir(lockFilePath), 0o755); err != nil {
		return nil, err
	}

	return osutil.NewFileLockWithMode(lockFilePath, 0o644)
}

func lockWithTimeout(l *osutil.FileLock, timeout time.Duration) error {
	startTime := time.Now()
	systemdWasNotified := false
	for {
		err := l.TryLock()
		if err != osutil.ErrAlreadyLocked {
			// We return nil if err is nil (that is, if we got the lock); we
			// also return for any error except for ErrAlreadyLocked, because
			// in that case we want to continue trying.
			return err
		}

		// The state is locked. Let's notify systemd that our startup might be
		// longer than usual, or we risk getting killed if we overstep the
		// systemd timeout.
		if !systemdWasNotified {
			logger.Noticef("Adjusting startup timeout by %v", timeout)
			systemdSdNotify(fmt.Sprintf("EXTEND_TIMEOUT_USEC=%d", timeout.Microseconds()))
			systemdWasNotified = true
		}

		if time.Since(startTime) >= timeout {
			return errors.New("timeout for state lock file expired")
		}
		time.Sleep(stateLockRetryInterval)
	}
}

func (o *Overlord) loadState(statePath string, restartHandler restart.Handler, backend state.Backend) (*state.State, error) {
	flock, err := initStateFileLock()
	if err != nil {
		return nil, fmt.Errorf("fatal: error opening lock file: %v", err)
	}
	o.stateFLock = flock

	logger.Noticef("Acquiring state lock file")
	if err := lockWithTimeout(o.stateFLock, stateLockTimeout); err != nil {
		logger.Noticef("Failed to lock state file")
		return nil, fmt.Errorf("fatal: could not lock state file: %v", err)
	}
	logger.Noticef("Acquired state lock file")

	curBootID, err := osutil.BootID()
	if err != nil {
		return nil, fmt.Errorf("fatal: cannot find current boot ID: %w", err)
	}
	// If workshop is PID 1 we don't care about /proc/sys/kernel/random/boot_id
	// as we are most likely running in a container. LXD mounts it's own boot_id
	// to correctly emulate the boot_id behaviour of non-containerized systems.
	// Within containerd/docker, boot_id is consistent with the host, which provides
	// us no context of restarts, so instead fallback to /proc/sys/kernel/random/uuid.
	if os.Getpid() == 1 {
		curBootID, err = randutil.RandomKernelUUID()
		if err != nil {
			return nil, fmt.Errorf("fatal: cannot generate psuedo boot-id: %w", err)
		}
	}

	if !osutil.FileExists(statePath) {
		// fail fast, mostly interesting for tests, this dir is set up by workshop
		stateDir := filepath.Dir(statePath)
		if !osutil.IsDir(stateDir) {
			return nil, fmt.Errorf("fatal: directory %q must be present", stateDir)
		}
		s := state.New(backend)
		initRestart(s, curBootID, restartHandler)
		patch.Init(s)
		return s, nil
	}

	r, err := os.Open(statePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read the state file: %w", err)
	}
	defer r.Close()

	var s *state.State
	s, err = state.ReadState(backend, r)
	if err != nil {
		return nil, err
	}

	err = initRestart(s, curBootID, restartHandler)
	if err != nil {
		return nil, err
	}

	// one-shot migrations
	err = patch.Apply(s)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func initRestart(s *state.State, curBootID string, restartHandler restart.Handler) error {
	s.Lock()
	defer s.Unlock()
	return restart.Init(s, curBootID, restartHandler)
}

func (o *Overlord) ensureTimerSetup() {
	o.ensureLock.Lock()
	defer o.ensureLock.Unlock()
	o.ensureTimer = time.NewTimer(ensureInterval)
	o.ensureNext = time.Now().Add(ensureInterval)
	o.pruneTicker = time.NewTicker(pruneInterval)
}

func (o *Overlord) ensureTimerReset() time.Time {
	o.ensureLock.Lock()
	defer o.ensureLock.Unlock()
	now := time.Now()
	o.ensureTimer.Reset(ensureInterval)
	o.ensureNext = now.Add(ensureInterval)
	return o.ensureNext
}

func (o *Overlord) ensureBefore(d time.Duration) {
	o.ensureLock.Lock()
	defer o.ensureLock.Unlock()
	if o.ensureTimer == nil {
		panic("cannot use EnsureBefore before Overlord.Loop")
	}
	now := time.Now()
	next := now.Add(d)
	if next.Before(o.ensureNext) {
		o.ensureTimer.Reset(d)
		o.ensureNext = next
		return
	}

	if o.ensureNext.Before(now) {
		// timer already expired, it will be reset in Loop() and
		// next Ensure() will be called shortly.
		if !o.ensureTimer.Stop() {
			return
		}
		o.ensureTimer.Reset(0)
		o.ensureNext = now
	}
}

// Loop runs a loop in a goroutine to ensure the current state regularly through StateEngine Ensure.
func (o *Overlord) Loop() {
	o.ensureTimerSetup()
	o.loopTomb.Go(func() error {
		for {
			// TODO: pass a proper context into Ensure
			o.ensureTimerReset()
			// in case of errors engine logs them,
			// continue to the next Ensure() try for now
			o.stateEng.Ensure()
			o.ensureDidRun()
			pruneC := pruneTickerC(o.pruneTicker)
			select {
			case <-o.loopTomb.Dying():
				return nil
			case <-o.ensureTimer.C:
			case <-pruneC:
				st := o.State()
				st.Lock()
				st.Prune(o.startOfOperationTime, pruneWait, abortWait, pruneMaxChanges)
				st.Unlock()
			}
		}
	})
}

func (o *Overlord) ensureDidRun() {
	atomic.StoreInt32(&o.ensureRun, 1)
}

func (o *Overlord) CanStandby() bool {
	run := atomic.LoadInt32(&o.ensureRun)
	return run != 0
}

// Stop stops the ensure loop and the managers under the StateEngine.
func (o *Overlord) Stop() error {
	o.loopTomb.Kill(nil)
	err := o.loopTomb.Wait()
	o.stateEng.Stop()
	return err
}

func (o *Overlord) settle(timeout time.Duration, beforeCleanups func()) error {
	if err := o.StartUp(); err != nil {
		return err
	}

	func() {
		o.ensureLock.Lock()
		defer o.ensureLock.Unlock()
		if o.ensureTimer != nil {
			panic("cannot use Settle concurrently with other Settle or Loop calls")
		}
		o.ensureTimer = time.NewTimer(0)
	}()

	defer func() {
		o.ensureLock.Lock()
		defer o.ensureLock.Unlock()
		o.ensureTimer.Stop()
		o.ensureTimer = nil
	}()

	t0 := time.Now()
	done := false
	var errs []error
	for !done {
		if timeout > 0 && time.Since(t0) > timeout {
			err := fmt.Errorf("Settle is not converging")
			if len(errs) != 0 {
				return &ensureError{append(errs, err)}
			}
			return err
		}
		next := o.ensureTimerReset()
		err := o.stateEng.Ensure()
		switch ee := err.(type) {
		case nil:
		case *ensureError:
			errs = append(errs, ee.errs...)
		default:
			errs = append(errs, err)
		}
		o.stateEng.Wait()
		o.ensureLock.Lock()
		done = o.ensureNext.Equal(next)
		o.ensureLock.Unlock()
		if done {
			if beforeCleanups != nil {
				beforeCleanups()
				beforeCleanups = nil
			}
			// we should wait also for cleanup handlers
			st := o.State()
			st.Lock()
			for _, chg := range st.Changes() {
				if chg.IsReady() && !chg.IsClean() {
					done = false
					break
				}
			}
			st.Unlock()
		}
	}
	if len(errs) != 0 {
		return &ensureError{errs}
	}
	return nil
}

// Settle runs first a state engine Ensure and then wait for
// activities to settle. That's done by waiting for all managers'
// activities to settle while making sure no immediate further Ensure
// is scheduled. It then waits similarly for all ready changes to
// reach the clean state. Chiefly for tests. Cannot be used in
// conjunction with Loop. If timeout is non-zero and settling takes
// longer than timeout, returns an error.
func (o *Overlord) Settle(timeout time.Duration) error {
	return o.settle(timeout, nil)
}

// SettleObserveBeforeCleanups runs first a state engine Ensure and
// then wait for activities to settle. That's done by waiting for all
// managers' activities to settle while making sure no immediate
// further Ensure is scheduled. It then waits similarly for all ready
// changes to reach the clean state, but calls once the provided
// callback before doing that. Chiefly for tests. Cannot be used in
// conjunction with Loop. If timeout is non-zero and settling takes
// longer than timeout, returns an error.
func (o *Overlord) SettleObserveBeforeCleanups(timeout time.Duration, beforeCleanups func()) error {
	return o.settle(timeout, beforeCleanups)
}

// State returns the system state managed by the overlord.
func (o *Overlord) State() *state.State {
	return o.stateEng.State()
}

// StateEngine returns the state engine used by overlord.
func (o *Overlord) StateEngine() *StateEngine {
	return o.stateEng
}

// TaskRunner returns the shared task runner responsible for running
// tasks for all managers under the overlord.
func (o *Overlord) TaskRunner() *state.TaskRunner {
	return o.runner
}

func (o *Overlord) WorkshopBackend() workshop.Backend {
	return workshop.WorkshopBackend(o.State())
}

func (o *Overlord) WorkshopManager() *workshopstate.WorkshopManager {
	return o.workshopmgr
}

func (o *Overlord) CommandManager() *cmdstate.CommandManager {
	return o.commandmgr
}

func (o *Overlord) HookManager() *hookstate.HookManager {
	return o.hookmgr
}

func (o *Overlord) InterfaceManager() *ifacestate.InterfaceManager {
	return o.ifacemgr
}

func (o *Overlord) SdkManager() *sdkstate.SdkManager {
	return o.sdkmgr
}

func MockWorkshopBackend(b workshop.Backend) func() {
	workshopBackendOverride = b
	return func() {
		workshopBackendOverride = nil
	}
}

// Fake creates an Overlord without any managers and with a backend
// not using disk. Managers can be added with AddManager. For testing.
func Fake() *Overlord {
	return FakeWithState(nil)
}

// FakeWithState creates an Overlord without any managers and
// with a backend not using disk. Managers can be added with AddManager. For
// testing.
func FakeWithState(handleRestart func(restart.RestartType)) *Overlord {
	o := &Overlord{
		loopTomb: new(tomb.Tomb),
		inited:   false,
	}
	s := state.New(fakeBackend{o: o})
	o.stateEng = NewStateEngine(s)
	o.runner = state.NewTaskRunner(s)
	return o
}

// AddManager adds a manager to a fake overlord. It cannot be used for
// a normally initialized overlord those are already fully populated.
func (o *Overlord) AddManager(mgr StateManager) {
	if o.inited {
		panic("internal error: cannot add managers to a fully initialized Overlord")
	}
	o.addManager(mgr)
}

type fakeBackend struct {
	o *Overlord
}

func (mb fakeBackend) Checkpoint(data []byte) error {
	return nil
}

func (mb fakeBackend) EnsureBefore(d time.Duration) {
	mb.o.ensureLock.Lock()
	timer := mb.o.ensureTimer
	mb.o.ensureLock.Unlock()
	if timer == nil {
		return
	}

	mb.o.ensureBefore(d)
}

func (mb fakeBackend) RequestRestart(t restart.RestartType) {
	panic("SHOULD NOT BE REACHED")
}
