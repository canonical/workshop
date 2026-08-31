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

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/canonical/x-go/strutil"
)

type HealthCheck struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
	Code      string    `json:"code,omitempty"`
}

type Mount struct {
	Plug           PlugRef `json:"plug"`
	HostSource     string  `json:"host-source,omitempty"`
	WorkshopSource string  `json:"workshop-source,omitempty"`
	WorkshopTarget string  `json:"workshop-target"`
}

type Endpoint struct {
	Protocol string `json:"protocol"`
	Path     string `json:"path,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     uint16 `json:"port,omitempty"`
}

type Tunnel struct {
	Plug PlugRef  `json:"plug"`
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`
}

type Sdk struct {
	Name        string       `json:"name"`
	Version     string       `json:"version,omitempty"`
	Channel     string       `json:"channel"`
	Source      string       `json:"source"`
	Revision    string       `json:"revision"`
	BuiltAt     time.Time    `json:"built-at"`
	InstalledAt time.Time    `json:"installed-at"`
	Health      *HealthCheck `json:"health-check,omitempty"`
	Mounts      []*Mount     `json:"mounts,omitempty"`
	Tunnels     []*Tunnel    `json:"tunnels,omitempty"`
}

type Workshops struct {
	Workshops []*WorkshopInfo `json:"workshops"`
	Files     []*WorkshopFile `json:"files"`
}

type WorkshopInfo struct {
	ProjectId   string   `json:"project-id"`
	Name        string   `json:"name"`
	Base        string   `json:"base"`
	Confinement string   `json:"confinement"`
	Status      string   `json:"status"`
	Sdks        []*Sdk   `json:"sdks,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}

type WorkshopFile struct {
	ProjectId string `json:"project-id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

type Workshop struct {
	WorkshopInfo
	Path string `json:"path"`
}

type Action struct {
	Script string `json:"script"`
}

type ListOptions struct {
	ProjectId string
}

type Remount struct {
	Action     string   `json:"action"`
	Plug       *PlugRef `json:"plug"`
	HostSource string   `json:"host-source"`
}

type SingleWorkshopError struct {
	Project string
	Names   []string
}

func (e *SingleWorkshopError) Error() string {
	if len(e.Names) == 0 {
		return fmt.Sprintf("no workshops found in %q", e.Project)
	}
	return fmt.Sprintf("multiple workshops found: %s", strutil.Quoted(e.Names))
}

func (client *Client) List(opts *ListOptions) ([]*WorkshopInfo, []*WorkshopFile, error) {
	query := url.Values{}
	query.Set("state", "available")
	var info Workshops
	_, err := client.doSync("GET", "/v1/projects/"+opts.ProjectId+"/workshops", query, nil, nil, &info)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot list workshops: %w", err)
	}
	return info.Workshops, info.Files, nil
}

func (client *Client) Workshop(projectId, name string) (*Workshop, error) {
	var workshop Workshop
	_, err := client.doSync("GET", "/v1/projects/"+projectId+"/workshops/"+name, nil, nil, nil, &workshop)
	if err != nil {
		return nil, err
	}
	return &workshop, nil
}

func (client *Client) SingleWorkshopName(project *Project) (string, error) {
	info, file, err := client.singleWorkshopOrFile(project)
	if err != nil {
		return "", fmt.Errorf("cannot infer workshop name: %w", err)
	}

	if info != nil {
		return info.Name, nil
	}
	if file != nil {
		return file.Name, nil
	}
	return "", errors.New("internal error: singleWorkshopOrFile returned nothing")
}

func (client *Client) SingleWorkshop(project *Project) (*Workshop, error) {
	info, file, err := client.singleWorkshopOrFile(project)
	if err != nil {
		return nil, fmt.Errorf("cannot infer workshop name: %w", err)
	}

	if info == nil {
		return nil, errors.New("workshop not launched")
	}
	workshop := Workshop{WorkshopInfo: *info}
	if file != nil {
		workshop.Path = file.Path
	}
	return &workshop, nil
}

func (client *Client) singleWorkshopOrFile(project *Project) (*WorkshopInfo, *WorkshopFile, error) {
	var info Workshops
	_, err := client.doSync("GET", "/v1/projects/"+project.Id+"/workshops", nil, nil, nil, &info)
	if err != nil {
		return nil, nil, err
	}

	var names []string
	for _, workshop := range info.Workshops {
		names = append(names, workshop.Name)
	}
	for _, file := range info.Files {
		if !slices.Contains(names, file.Name) {
			names = append(names, file.Name)
		}
	}

	if len(names) != 1 {
		return nil, nil, &SingleWorkshopError{project.Path, names}
	}

	var workshop *WorkshopInfo
	if len(info.Workshops) > 0 {
		workshop = info.Workshops[0]
	}
	var file *WorkshopFile
	if len(info.Files) > 0 {
		file = info.Files[0]
	}
	return workshop, file, nil
}

func (client *Client) ListActions(projectId, name string) (map[string]Action, error) {
	var actions map[string]Action
	_, err := client.doSync("GET", "/v1/projects/"+projectId+"/workshops/"+name+"/actions", nil, nil, nil, &actions)
	if err != nil {
		return nil, err
	}
	return actions, nil
}

func (client *Client) Remount(plug *PlugRef, source string) (changeId string, err error) {
	var body bytes.Buffer
	var remoutReq = Remount{
		Action:     "remount",
		Plug:       plug,
		HostSource: source,
	}
	if err := json.NewEncoder(&body).Encode(remoutReq); err != nil {
		return "", err
	}

	return client.doAsync("POST", "/v1/projects/"+plug.ProjectId+"/workshops/"+plug.Workshop+"/mounts", nil, nil, &body)
}
