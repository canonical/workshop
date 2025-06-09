package lxdbackend

import (
	"cmp"
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

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	if err := s.stopWorkshop(conn, ctx, name, true); err != nil {
		return err
	}

	rev.Add(func() {
		if rerr := s.startWorkshop(conn, ctx, name); rerr != nil {
			logger.Debugf("On StashWorkshop: Cannot restart %q workshop after failed stash operation: %v", name, rerr)
		}
	})

	user, ok := ctx.Value(workshop.ContextUser).(string)
	if !ok {
		return fmt.Errorf("context key %s not found", workshop.ContextUser)
	}

	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	instance := InstanceName(name, projectId)
	stashed := instanceStashName(name, projectId)

	sourceProject, err1 := lxdProjectName(user)
	targetProject, err2 := lxdStashProjectName(user)
	if err = cmp.Or(err1, err2); err != nil {
		return err
	}

	if err = s.copyInstance(conn, instance, stashed, sourceProject, targetProject); err != nil {
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

	conn, err := s.LxdClient(ctx)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	instance := InstanceName(name, projectId)
	stash := instanceStashName(name, projectId)

	sourceProject, err1 := lxdStashProjectName(user)
	targetProject, err2 := lxdProjectName(user)
	if err = cmp.Or(err1, err2); err != nil {
		return err
	}

	if err := s.copyInstance(conn, stash, instance, sourceProject, targetProject); err != nil {
		return err
	}

	return s.startWorkshop(conn, ctx, name)
}

// Copies the instance between LXD projects.
func (s *Backend) copyInstance(conn lxd.InstanceServer, srcName, dstName, sourceProject, targetProject string) error {
	conn = conn.UseProject(sourceProject)
	instance, _, err := conn.GetInstance(srcName)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return workshop.ErrWorkshopNotLaunched
		}
		return err
	}

	dest := conn.UseProject(targetProject)
	rop, err := dest.CopyInstance(conn, *instance, &lxd.InstanceCopyArgs{Name: dstName})
	if err != nil {
		return err
	}
	if err = rop.Wait(); err != nil {
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

	stash, err := lxdStashProjectName(user)
	if err != nil {
		return err
	}

	conn = conn.UseProject(stash)
	iname := instanceStashName(name, projectId)

	op, err := conn.DeleteInstance(iname)
	if err != nil {
		return err
	}
	if err = op.Wait(); err != nil {
		return err
	}
	return nil
}
