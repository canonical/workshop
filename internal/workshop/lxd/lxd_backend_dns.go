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

package lxdbackend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"golang.org/x/net/idna"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/workshop"
)

// cname represents a DNS CNAME record for a workshop. By default, workshops
// register themselves as <Workshop>-<ProjectId>.wp via DHCP. After starting a
// workshop, the LXD backend configures dnsmasq with these CNAME records:
// - <Workshop>.<ProjectId>.wp
// - <Workshop>.<ProjectAlias>.wp
// Both records point to <Workshop>-<ProjectId>.wp.
//
// ProjectName is the user-facing project name, ideally the basename of the
// project directory. In these cases we fall back to the project ID:
// - The basename is invalid according to golang.org/x/net/idna.
// - The basename is already used by another project.
// To support common project names, we replace dots, spaces and underscores
// with hyphens before validation.
//
// ProjectAlias is the punycode ASCII encoding of ProjectName. The reason for
// this is that dnsmasq (via libidn2) is stricter than Go about valid domain
// names. It still allows some unicode characters, but not as many. If
// validation fails, LXD will roll back the changes to raw.dnsmasq so we don't
// even get the ProjectId fallback. If ProjectName != ProjectAlias, we store
// the ProjectName as a comment so that `workshop info` can display it.
//
// Once all workshops in a given project are stopped manually, ProjectName is
// made available, but only for newly started workshops. If a workshop is
// stopped using lxc or by other means, the project name remains in use until
// it is stopped normally, or the project is pruned.
//
// Note is an optional human-readable reason why the basename wasn't used,
// stored as a comment in the dnsmasq config.
type cname struct {
	Workshop     string
	ProjectId    string
	ProjectName  string
	ProjectAlias string
	Note         string
}

var (
	dnsmasqLock  sync.Mutex
	dnsmasqGuard struct {
		c       chan struct{}
		counter int32
	}
)

func (s *Backend) addWorkshopCNAMEs(conn lxd.InstanceServer, ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	// Call this before locking because it might prune the dnsmasq config.
	projects, err := s.UserProjects(ctx)
	if err != nil {
		return err
	}

	if err := lockDnsmasq(ctx); err != nil {
		return err
	}
	defer unlockDnsmasq()

	network, etag, err := conn.GetNetwork(networkName)
	if err != nil {
		return err
	}

	cnames, lines := unmarshalDnsmasq(network.Config["raw.dnsmasq"])

	entry, err := generateCNAME(cnames, projects, projectId, name)
	if err != nil {
		return err
	}

	idx := slices.IndexFunc(cnames, func(c cname) bool {
		return c.Workshop == entry.Workshop && c.ProjectId == entry.ProjectId
	})
	if idx < 0 {
		cnames = append(cnames, entry)
	} else if cnames[idx] == entry {
		return nil
	} else {
		cnames[idx] = entry
	}

	if network.Config == nil {
		network.Config = make(map[string]string, 1)
	}
	network.Config["raw.dnsmasq"] = marshalDnsmasq(cnames, lines)

	op, err := conn.UpdateNetwork(network.Name, network.Writable(), etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

func generateCNAME(cnames []cname, projects []workshop.Project, projectId string, name string) (cname, error) {
	result := cname{Workshop: name, ProjectId: projectId, ProjectName: projectId, ProjectAlias: projectId}

	idx := slices.IndexFunc(cnames, func(c cname) bool {
		return c.ProjectId != projectId && strings.EqualFold(c.ProjectAlias, projectId)
	})
	if idx >= 0 {
		return cname{}, fmt.Errorf("hostname %s.%s already taken by %s", projectId, networkDomain, cnames[idx].friendly())
	}

	idx = slices.IndexFunc(projects, func(p workshop.Project) bool {
		return p.ProjectId == projectId
	})
	if idx < 0 {
		return cname{}, fmt.Errorf("project %q not found", projectId)
	}

	projectName := filepath.Base(projects[idx].Path)
	projectName = strings.ReplaceAll(projectName, " ", "-")
	projectName = strings.ReplaceAll(projectName, "_", "-")
	projectName = strings.ReplaceAll(projectName, ".", "-")

	projectAlias, err := idna.Lookup.ToASCII(projectName)
	if err != nil {
		logger.Noticef("On StartWorkshop: invalid domain name %q, using %q instead", projectName+"."+networkDomain, result.friendly())
		result.Note = "hostname-fallback"
		return result, nil //nolint:nilerr
	}

	conflict := slices.ContainsFunc(cnames, func(c cname) bool {
		if c.ProjectId == projectId {
			return false
		}
		return strings.EqualFold(c.ProjectId, projectAlias) || strings.EqualFold(c.ProjectAlias, projectAlias)
	})
	if conflict {
		logger.Noticef("On StartWorkshop: domain name %q in use, using %q instead", projectAlias+"."+networkDomain, result.friendly())
		result.Note = "hostname-fallback"
		return result, nil
	}

	result.ProjectName = projectName
	result.ProjectAlias = projectAlias
	return result, nil
}

func (s *Backend) removeWorkshopCNAMEs(conn lxd.InstanceServer, ctx context.Context, name string) error {
	projectId, ok := ctx.Value(workshop.ContextProjectId).(string)
	if !ok {
		return fmt.Errorf("context key project-id not found")
	}

	if err := lockDnsmasq(ctx); err != nil {
		return err
	}
	defer unlockDnsmasq()

	network, etag, err := conn.GetNetwork(networkName)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil
		}
		return err
	}

	cnames, lines := unmarshalDnsmasq(network.Config["raw.dnsmasq"])

	entries := len(cnames)
	cnames = slices.DeleteFunc(cnames, func(c cname) bool {
		return c.Workshop == name && c.ProjectId == projectId
	})
	if entries == len(cnames) {
		return nil
	}

	network.Config["raw.dnsmasq"] = marshalDnsmasq(cnames, lines)

	op, err := conn.UpdateNetwork(network.Name, network.Writable(), etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (s *Backend) pruneWorkshopCNAMEs(conn lxd.InstanceServer, ctx context.Context, removed []workshop.Project) error {
	if len(removed) == 0 {
		return nil
	}

	if err := lockDnsmasq(ctx); err != nil {
		return err
	}
	defer unlockDnsmasq()

	network, etag, err := conn.GetNetwork(networkName)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return nil
		}
		return err
	}

	cnames, lines := unmarshalDnsmasq(network.Config["raw.dnsmasq"])

	projects := make(map[string]struct{}, len(removed))
	for _, p := range removed {
		projects[p.ProjectId] = struct{}{}
	}

	entries := len(cnames)
	cnames = slices.DeleteFunc(cnames, func(c cname) bool {
		_, ok := projects[c.ProjectId]
		return ok
	})
	if entries == len(cnames) {
		return nil
	}

	network.Config["raw.dnsmasq"] = marshalDnsmasq(cnames, lines)

	op, err := conn.UpdateNetwork(network.Name, network.Writable(), etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

func lockDnsmasq(ctx context.Context) error {
	var locked <-chan struct{}

	dnsmasqLock.Lock()
	if dnsmasqGuard.c == nil {
		dnsmasqGuard.c = make(chan struct{}, 1)
		dnsmasqGuard.c <- struct{}{}
	}
	dnsmasqGuard.counter += 1
	locked = dnsmasqGuard.c
	dnsmasqLock.Unlock()

	select {
	case <-locked:
		return nil
	case <-ctx.Done():
		dnsmasqLock.Lock()
		dnsmasqGuard.counter -= 1
		if dnsmasqGuard.counter == 0 {
			close(dnsmasqGuard.c)
			dnsmasqGuard.c = nil
		}
		dnsmasqLock.Unlock()
		return ctx.Err()
	}
}

func unlockDnsmasq() {
	dnsmasqLock.Lock()
	defer dnsmasqLock.Unlock()

	dnsmasqGuard.c <- struct{}{}

	dnsmasqGuard.counter -= 1
	if dnsmasqGuard.counter == 0 {
		close(dnsmasqGuard.c)
		dnsmasqGuard.c = nil
	}
}

func marshalDnsmasq(cnames []cname, lines []string) string {
	combined := make([]string, 0, len(cnames)+len(lines))
	for _, c := range cnames {
		combined = append(combined, "cname="+c.String())
	}
	combined = append(combined, lines...)
	return strings.Join(combined, "\n")
}

func unmarshalDnsmasq(config string) ([]cname, []string) {
	var cnames []cname
	var lines []string
	for line := range strings.SplitSeq(config, "\n") {
		lines = append(lines, line)

		option, ok := strings.CutPrefix(line, "cname=")
		if !ok {
			continue
		}
		var c cname
		if err := c.UnmarshalText([]byte(option)); err != nil {
			continue
		}

		cnames = append(cnames, c)
		lines = lines[:len(lines)-1]
	}
	return cnames, lines
}

func (c cname) String() string {
	alias := c.Workshop + "." + c.ProjectId + "." + networkDomain
	target := InstanceName(c.Workshop, c.ProjectId) + "." + networkDomain
	ttl := "0"

	values := []string{alias, target, ttl}
	var comment string
	if c.ProjectAlias == c.ProjectId {
		comment = c.Note
	} else {
		friendlyAlias := c.Workshop + "." + c.ProjectAlias + "." + networkDomain
		values = []string{alias, friendlyAlias, target, ttl}
		if c.ProjectName != c.ProjectAlias {
			comment = c.ProjectName
		}
	}

	result := strings.Join(values, ",")
	if comment != "" {
		result += "  # "
		result += comment
	}
	return result
}

func (c cname) friendly() string {
	return c.Workshop + "." + c.ProjectName + "." + networkDomain
}

func (c cname) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

func (c *cname) UnmarshalText(text []byte) error {
	value, comment, _ := strings.Cut(string(text), "  # ")

	values := strings.Split(value, ",")
	if len(values) < 3 {
		return errors.New("not enough arguments")
	}
	if len(values) > 4 {
		return errors.New("too many arguments")
	}
	if values[len(values)-1] != "0" {
		return errors.New("invalid TTL")
	}

	target, ok := strings.CutSuffix(values[len(values)-2], "."+networkDomain)
	if !ok {
		return errors.New("invalid top-level domain")
	}
	idx := strings.LastIndexByte(target, '-')
	if idx < 0 {
		return errors.New("invalid hostname")
	}
	c.Workshop, c.ProjectId = target[:idx], target[idx+1:]
	if err := workshop.ValidateProjectId(c.ProjectId); err != nil {
		return err
	}

	if len(values) == 3 {
		c.ProjectName = c.ProjectId
		c.ProjectAlias = c.ProjectId
		c.Note = comment
	} else {
		prefix, ok := strings.CutSuffix(values[1], "."+networkDomain)
		if !ok {
			return errors.New("invalid alias")
		}
		c.ProjectAlias, ok = strings.CutPrefix(prefix, c.Workshop+".")
		if !ok {
			return errors.New("invalid alias")
		}
		if comment == "" {
			c.ProjectName = c.ProjectAlias
		} else {
			c.ProjectName = comment
		}
		c.Note = ""
	}

	if c.String() != string(text) {
		return errors.New("invalid alias")
	}
	return nil
}
