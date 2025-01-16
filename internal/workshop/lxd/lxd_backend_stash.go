package lxdbackend

import (
	"context"
	"fmt"
	"net/http"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/revert"
	"github.com/canonical/workshop/internal/workshop"
)

func (s *Backend) StashWorkshop(ctx context.Context, name string) error {
	rev := revert.New()
	defer rev.Fail()

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	instance := InstanceName(name, projectId)
	stashedInsance := workshop.StashNamePrefix + instance
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	if err := s.updateInstanceState(conn, ctx, name, "stop", true); err != nil {
		return err
	}

	rev.Add(func() {
		err := s.updateInstanceState(conn, ctx, name, "start", false)
		if err != nil {
			logger.Debugf("Cannot restart %q workshop after failed stash operation", name)
		}
	})

	if err = s.moveInstanceAndProfiles(conn, instance, stashedInsance, LxdProjectName(user), LxdSystemProjectName(user)); err != nil {
		return err
	}

	rev.Success()
	return nil
}

func (s *Backend) UnstashWorkshop(ctx context.Context, name string) error {
	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	instance := InstanceName(name, projectId)
	stashedInsance := workshop.StashNamePrefix + instance
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	if err := s.moveInstanceAndProfiles(conn, stashedInsance, instance, LxdSystemProjectName(user), LxdProjectName(user)); err != nil {
		return err
	}

	if err := s.updateInstanceState(conn, ctx, name, "start", false); err != nil {
		return err
	}
	return nil
}

// Moves the instance between the project and stash area (or the other way around)
// instanceFrom - the instance's source name
// instanceTo - the instance's dest name (must be different due to LXD DNS conflicts)
// source - the LXD project name to move instance from
// target - the LXD project name to move instance to
func (s *Backend) moveInstanceAndProfiles(conn lxd.InstanceServer, instanceFrom, instanceTo, source, target string) error {
	conn = conn.UseProject(source)
	_, _, err := conn.GetInstance(instanceFrom)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return workshop.ErrWorkshopNotLaunched
		}
		return err
	}
	// Stash the workshop
	// the new name must not be the same, otherwise the LXD's DNS will fail
	// the new instance creation; hence, the prefix.
	if op, err := conn.MigrateInstance(instanceFrom, api.InstancePost{
		Name:      instanceTo,
		Project:   target,
		Migration: true,
		Profiles:  []string{"default"},
	}); err != nil {
		return err
	} else if err = op.Wait(); err != nil {
		return err
	}
	return nil
}

func (s *Backend) RemoveWorkshopStash(ctx context.Context, name string) error {
	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	conn = conn.UseProject(LxdSystemProjectName(user))
	iname := workshop.StashNamePrefix + InstanceName(name, projectId)

	// 1. Remove the workshop instance
	op, err := conn.DeleteInstance(iname)
	if err != nil {
		return err
	} else if err = op.WaitContext(ctx); err != nil {
		return nil
	}
	return nil
}
