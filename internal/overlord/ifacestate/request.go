package ifacestate

import (
	"fmt"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/conflict"
	"github.com/canonical/workshop/internal/overlord/healthstate"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/workshop"
)

// ErrAlreadyConnected describes the error that occurs when attempting to connect already connected interface.
type ErrAlreadyConnected struct {
	Connection interfaces.ConnRef
}

func (e ErrAlreadyConnected) Error() string {
	return fmt.Sprintf("already connected: %q", e.Connection.ID())
}

// Connect returns a set of tasks for connecting an interface.
func Connect(st *state.State, plugW *workshop.Workshop, slotW *workshop.Workshop, connRef *interfaces.ConnRef) (*state.TaskSet, error) {
	allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.WaitingStatus, healthstate.StoppedStatus}
	if err := healthstate.CheckWorkshopHealth(st, plugW, allowed); err != nil {
		return nil, fmt.Errorf("cannot connect %q: %w", connRef.PlugRef.ShortRef(), err)
	}
	if err := healthstate.CheckWorkshopHealth(st, slotW, allowed); err != nil {
		return nil, fmt.Errorf("cannot connect %q: %w", connRef.SlotRef.ShortRef(), err)
	}

	if _, err := conflict.FindChangeByKind(st, plugW.Project.ProjectId, plugW.Name, "exec"); err == nil {
		st.Warnf("active shell sessions in %q may need restarting for new connections to take effect", plugW.Name)
	}
	if _, err := conflict.FindChangeByKind(st, slotW.Project.ProjectId, slotW.Name, "exec"); err == nil {
		st.Warnf("active shell sessions in %q may need restarting for new connections to take effect", slotW.Name)
	}

	// check if the connection already exists
	conns, err := getConns(st)
	if err != nil {
		return nil, err
	}
	if conn, ok := conns[connRef.ID()]; ok && !conn.Undesired {
		return nil, &ErrAlreadyConnected{Connection: *connRef}
	}

	master, affected := MaybeBound(plugW, connRef.PlugRef)
	masterTask := st.NewTask("connect", fmt.Sprintf("Connect %q to %q", master.ShortRef(), connRef.SlotRef.ShortRef()))

	masterTask.Set("slot", connRef.SlotRef)
	masterTask.Set("plug", master)
	masterTask.Set("delayed-setup-profile", false)

	// master's plug connection that every other connection from affected will
	// be bound to.
	bref := interfaces.ConnRef{PlugRef: master, SlotRef: connRef.SlotRef}

	ts := state.NewTaskSet(masterTask)
	prev := masterTask
	for _, p := range affected {
		slave := st.NewTask("connect", fmt.Sprintf("Connect %q to %q", p.ShortRef(), connRef.SlotRef.ShortRef()))
		slave.Set("slot", connRef.SlotRef)
		slave.Set("plug", p)
		slave.Set("delayed-setup-profile", true)

		slave.Set("plug-dynamic", map[string]interface{}{"bind": bref.ID()})

		slave.WaitFor(prev)
		prev = slave
		ts.AddTask(slave)
	}

	return ts, nil
}

func maybeAddDisconnect(st *state.State, ts *state.TaskSet, conn interfaces.ConnRef, forget bool, seen map[interfaces.ConnRef]bool) {
	if _, ok := seen[conn]; ok {
		return
	}

	dtask := st.NewTask("disconnect", fmt.Sprintf("Disconnect %q from %q", conn.PlugRef.ShortRef(), conn.SlotRef.ShortRef()))
	dtask.Set("plug", conn.PlugRef)
	dtask.Set("slot", conn.SlotRef)
	dtask.Set("forget", forget)

	l := len(ts.Tasks())
	if l > 0 {
		dtask.WaitFor(ts.Tasks()[l-1])
	}
	ts.AddTask(dtask)
	seen[conn] = true
}

func disconnect(st *state.State, plugW *workshop.Workshop, conn *interfaces.ConnRef, forget bool, seen map[interfaces.ConnRef]bool) (*state.TaskSet, error) {
	master, affected := MaybeBound(plugW, conn.PlugRef)
	var ts = state.NewTaskSet()

	cref := interfaces.ConnRef{PlugRef: master, SlotRef: conn.SlotRef}
	maybeAddDisconnect(st, ts, cref, forget, seen)

	for _, slave := range affected {
		cref = interfaces.ConnRef{PlugRef: slave, SlotRef: conn.SlotRef}
		maybeAddDisconnect(st, ts, cref, forget, seen)
	}
	return ts, nil
}

// Disconnect returns a set of tasks for disconnecting an interface.
func Disconnect(st *state.State, plugW *workshop.Workshop, slotW *workshop.Workshop, conn *interfaces.ConnRef, forget bool, seen map[interfaces.ConnRef]bool) (*state.TaskSet, error) {
	allowed := []healthstate.Status{healthstate.ReadyStatus, healthstate.WaitingStatus, healthstate.StoppedStatus}
	if err := healthstate.CheckWorkshopHealth(st, plugW, allowed); err != nil {
		return nil, fmt.Errorf("cannot disconnect %q: %w", conn.PlugRef.ShortRef(), err)
	}
	if err := healthstate.CheckWorkshopHealth(st, slotW, allowed); err != nil {
		return nil, fmt.Errorf("cannot disconnect %q: %w", conn.SlotRef.ShortRef(), err)
	}

	return disconnect(st, plugW, conn, forget, seen)
}
