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

type Sdk struct {
	Name        string       `json:"name"`
	Version     string       `json:"version,omitempty"`
	Channel     string       `json:"channel"`
	Revision    string       `json:"revision"`
	BuildTime   time.Time    `json:"build-time"`
	InstallTime time.Time    `json:"install-time"`
	Health      *HealthCheck `json:"health-check,omitempty"`
	Mounts      []*Mount     `json:"mounts,omitempty"`
}

type Workshops struct {
	Workshops []*WorkshopInfo `json:"workshops"`
	Files     []*WorkshopFile `json:"files"`
}

type WorkshopInfo struct {
	ProjectId string   `json:"project-id"`
	Name      string   `json:"name"`
	Base      string   `json:"base"`
	Status    string   `json:"status"`
	Sdks      []*Sdk   `json:"sdks,omitempty"`
	Notes     []string `json:"notes,omitempty"`
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

type Script struct {
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

	if len(names) < 1 {
		return nil, nil, fmt.Errorf("no workshops found in %q", project.Path)
	}
	if len(names) > 1 {
		return nil, nil, fmt.Errorf("multiple workshops found: %s", strutil.Quoted(names))
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

func (client *Client) ListScripts(projectId, name string) (map[string]Script, error) {
	var scripts map[string]Script
	_, err := client.doSync("GET", "/v1/projects/"+projectId+"/workshops/"+name+"/scripts", nil, nil, nil, &scripts)
	if err != nil {
		return nil, err
	}
	return scripts, nil
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
