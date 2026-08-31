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

package sdk

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/metautil"
	"github.com/canonical/workshop/internal/timeutil"
)

type Meta struct {
	Setup
	SdkYAML string
}

type Setup struct {
	Name      string   `json:"name"`
	PackageID string   `json:"package-id,omitempty"`
	Channel   string   `json:"channel,omitempty"`
	Source    Source   `json:"source,omitempty"`
	Revision  Revision `json:"revision"`
	Sha3_384  string   `json:"sha3-384"`
}

func (s *Setup) Filepath() string {
	return filepath.Join(dirs.SdkDownloads, s.filename())
}

func (s *Setup) filename() string {
	return fmt.Sprintf("%s_%s.sdk", s.Name, s.Revision.String())
}

func (s *Setup) IsVolume() bool {
	return s.Source == StoreSource || s.Source == SystemSource || s.Source == TrySource
}

func VolumeName(name string, revision Revision) string {
	return fmt.Sprintf("%s-%s", name, revision)
}

type Source int

const (
	StoreSource Source = iota
	SystemSource
	TrySource
	ProjectSource
	SketchSource
)

func (s Source) MarshalText() ([]byte, error) {
	var text string
	switch s {
	case StoreSource:
		text = "store"
	case SystemSource:
		text = "system"
	case TrySource:
		text = "try"
	case ProjectSource:
		text = "project"
	case SketchSource:
		text = "sketch"
	default:
		return nil, fmt.Errorf("invalid SDK source: %v", int(s))
	}
	return []byte(text), nil
}

func (s *Source) UnmarshalText(text []byte) error {
	switch string(text) {
	case "store":
		*s = StoreSource
	case "system":
		*s = SystemSource
	case "try":
		*s = TrySource
	case "project":
		*s = ProjectSource
	case "sketch":
		*s = SketchSource
	default:
		return fmt.Errorf("invalid SDK source: %q", string(text))
	}
	return nil
}

func (s Source) NeedsRetrieve() bool {
	return s == StoreSource || s == SystemSource
}

// ContentID contains the information essential to identifying an SDK. SDKs
// with the same ContentID will behave the same when installed in a workshop,
// even if they come from different channels or sources.
// It is not the same as the SDK Store PackageID.
type ContentID struct {
	Name     string
	Sha3_384 string
	IsVolume bool
}

func SetupContentID(setup Setup) ContentID {
	return ContentID{Name: setup.Name, Sha3_384: setup.Sha3_384, IsVolume: setup.IsVolume()}
}

type sdkYaml struct {
	Name        string            `yaml:"name"`
	Base        string            `yaml:"base"`
	Arch        string            `yaml:"architecture"`
	Version     string            `yaml:"version,omitempty"`
	Title       string            `yaml:"title"`
	Summary     string            `yaml:"summary"`
	Description string            `yaml:"description"`
	License     string            `yaml:"license"`
	Type        string            `yaml:"type"`
	BuiltAt     *timeutil.TimeUTC `yaml:"sdkcraft-started-at,omitempty"`
	Plugs       map[string]any    `yaml:"plugs,omitempty"`
	Slots       map[string]any    `yaml:"slots,omitempty"`
}

// SketchSDKYaml describes the editable YAML shape of a sketch SDK.
type SketchSDKYaml struct {
	Description string            `yaml:"description"`
	Hooks       map[string]string `yaml:"hooks,omitempty"`
	Name        string            `yaml:"name"`
	Plugs       map[string]any    `yaml:"plugs,omitempty"`
	Slots       map[string]any    `yaml:"slots,omitempty"`
	Summary     string            `yaml:"summary"`
	Title       string            `yaml:"title"`
}

type Type string

const Sketch = "sketch"

const (
	Regular Type = "regular"
	System  Type = "system"
)

func (t Type) String() string {
	return string(t)
}

func IsSystem(name string) bool {
	return name == System.String()
}

func IsSketch(name string) bool {
	return name == Sketch
}

type Info struct {
	ProjectId   string
	Workshop    string
	Name        string
	PackageID   string
	Base        string
	Arch        string
	Version     string
	Type        Type
	Revision    Revision
	Channel     string
	Source      Source
	BuiltAt     *time.Time
	Title       string
	Summary     string
	Description string
	License     string

	Plugs     map[string]*PlugInfo
	PlugBinds map[string]PlugRef
	Slots     map[string]*SlotInfo
	// Plugs or slots with issues (they are not included in Plugs or Slots)
	BadInterfaces map[string]string
}

func (i *Info) Ref() Ref {
	return Ref{
		ProjectId: i.ProjectId,
		Workshop:  i.Workshop,
		Sdk:       i.Name,
	}
}

func (i *Info) SetupPlugBinds(binds map[string]PlugRef) error {
	for name, plug := range binds {
		if _, ok := i.Plugs[name]; ok {
			// Check plugs that are bound. The existence of plugs that are
			// "bound to" it will be checked at the connecting stage, i.e. when
			// all plugs from all SDKs are in the repository already.
			i.PlugBinds[name] = plug
		} else {
			return fmt.Errorf("plug binding failed: SDK %q has no plug named %q", i.Ref().ShortRef(), name)
		}
	}
	return nil
}

// Adds slots defined for this SDK in a workshop file.
func (i *Info) SetupWorkshopSlots(slots map[string]any) error {
	for name, data := range slots {
		if _, exist := i.Slots[name]; exist {
			return fmt.Errorf("cannot add slot %q to %q SDK: already exists", name, i.Name)
		}
		iface, label, attrs, err := convertToSlotOrPlugData("slot", name, data)
		if err != nil {
			return err
		}
		i.Slots[name] = &SlotInfo{
			Sdk:       i,
			Name:      name,
			Interface: iface,
			Attrs:     attrs,
			Label:     label,
		}
	}

	SanitizePlugsSlots(i)
	return nil
}

// Adds slots defined for this SDK in a workshop file.
func (i *Info) SetupWorkshopPlugs(plugs map[string]any) error {
	for name, data := range plugs {
		if _, exist := i.Plugs[name]; exist {
			return fmt.Errorf("cannot add plug %q to %q SDK: already exists", name, i.Name)
		}
		iface, label, attrs, err := convertToSlotOrPlugData("plug", name, data)
		if err != nil {
			return err
		}
		i.Plugs[name] = &PlugInfo{
			Sdk:       i,
			Name:      name,
			Interface: iface,
			Attrs:     attrs,
			Label:     label,
		}
	}

	SanitizePlugsSlots(i)
	return nil
}

type Ref struct {
	ProjectId string
	Workshop  string
	Sdk       string
}

func (r Ref) String() string {
	return fmt.Sprintf("%s/%s/%s", r.ProjectId, r.Workshop, r.Sdk)
}

func (r Ref) ShortRef() string {
	return fmt.Sprintf("%s/%s", r.Workshop, r.Sdk)
}

var SanitizePlugsSlots = func(snapInfo *Info) {
	panic("SanitizePlugsSlots function not set")
}

func ReadSdkInfo(yamlData []byte, projectId, workshop string) (*Info, error) {
	var sdkYaml sdkYaml
	err := yaml.Unmarshal(yamlData, &sdkYaml)
	if err != nil {
		return nil, err
	}

	if sdkYaml.Type == "" {
		sdkYaml.Type = Regular.String()
	}
	if sdkYaml.Type == System.String() && !IsSystem(sdkYaml.Name) {
		return nil, fmt.Errorf("type %q is reserved for the system SDK", sdkYaml.Type)
	}

	sdkInfo := &Info{
		ProjectId:     projectId,
		Workshop:      workshop,
		Name:          sdkYaml.Name,
		Base:          sdkYaml.Base,
		Arch:          sdkYaml.Arch,
		Version:       sdkYaml.Version,
		Type:          Type(sdkYaml.Type),
		BuiltAt:       (*time.Time)(sdkYaml.BuiltAt),
		Title:         sdkYaml.Title,
		Summary:       sdkYaml.Summary,
		Description:   sdkYaml.Description,
		License:       sdkYaml.License,
		Plugs:         make(map[string]*PlugInfo),
		PlugBinds:     make(map[string]PlugRef),
		Slots:         make(map[string]*SlotInfo),
		BadInterfaces: make(map[string]string),
	}

	if err := setPlugsFromSdkYaml(&sdkYaml, sdkInfo); err != nil {
		return nil, err
	}

	if err := setSlotsFromSdkYaml(&sdkYaml, sdkInfo); err != nil {
		return nil, err
	}

	SanitizePlugsSlots(sdkInfo)
	return sdkInfo, nil
}

func setPlugsFromSdkYaml(y *sdkYaml, sdk *Info) error {
	for name, data := range y.Plugs {
		iface, label, attrs, err := convertToSlotOrPlugData("plug", name, data)
		if err != nil {
			return err
		}
		sdk.Plugs[name] = &PlugInfo{
			Sdk:       sdk,
			Name:      name,
			Interface: iface,
			Attrs:     attrs,
			Label:     label,
		}
	}

	return nil
}

func setSlotsFromSdkYaml(y *sdkYaml, sdk *Info) error {
	for name, data := range y.Slots {
		iface, label, attrs, err := convertToSlotOrPlugData("slot", name, data)
		if err != nil {
			return err
		}
		sdk.Slots[name] = &SlotInfo{
			Sdk:       sdk,
			Name:      name,
			Interface: iface,
			Attrs:     attrs,
			Label:     label,
		}
	}

	return nil
}

func convertToSlotOrPlugData(plugOrSlot, name string, data any) (iface, label string, attrs map[string]any, err error) {
	iface = name
	switch data := data.(type) {
	case string:
		return data, "", nil, nil
	case nil:
		return name, "", nil, nil
	case map[string]any:
		for key, valueData := range data {
			if strings.HasPrefix(key, "$") {
				err := fmt.Errorf("%s %q uses reserved attribute %q", plugOrSlot, name, key)
				return "", "", nil, err
			}
			switch key {
			case "":
				return "", "", nil, fmt.Errorf("%s %q has an empty attribute key", plugOrSlot, name)
			case "interface":
				value, ok := valueData.(string)
				if !ok {
					err := fmt.Errorf("interface name on %s %q is not a string (found %T)",
						plugOrSlot, name, valueData)
					return "", "", nil, err
				}
				iface = value
			case "label":
				value, ok := valueData.(string)
				if !ok {
					err := fmt.Errorf("label of %s %q is not a string (found %T)",
						plugOrSlot, name, valueData)
					return "", "", nil, err
				}
				label = value
			default:
				if attrs == nil {
					attrs = make(map[string]any)
				}
				value, err := metautil.NormalizeValue(valueData)
				if err != nil {
					return "", "", nil, fmt.Errorf("attribute %q of %s %q: %v", key, plugOrSlot, name, err)
				}
				attrs[key] = value
			}
		}
		return iface, label, attrs, nil
	default:
		err := fmt.Errorf("%s %q has malformed definition (found %T)", plugOrSlot, name, data)
		return "", "", nil, err
	}
}

// SlotInfo provides information about a slot.
type SlotInfo struct {
	Sdk *Info

	Name      string
	Interface string
	Attrs     map[string]any
	Label     string
}

type AttributeNotFoundError struct {
	Attribute string
	Plug      *PlugRef
	Slot      *SlotRef
}

func (e *AttributeNotFoundError) Error() string {
	if e.Slot == nil {
		return fmt.Sprintf("attribute %q not found for plug %q", e.Attribute, e.Plug.ShortRef())
	}
	return fmt.Sprintf("attribute %q not found for slot %q", e.Attribute, e.Slot.ShortRef())

}

func (slot *SlotInfo) Attr(key string, val any) error {
	v, ok := slot.Lookup(key)
	if !ok {
		ref := slot.Ref()
		return &AttributeNotFoundError{Attribute: key, Slot: &ref}
	}

	if err := metautil.SetValueFromAttribute(v, val); err != nil {
		return fmt.Errorf("invalid attribute %q for slot %q: %w", key, slot.Ref().ShortRef(), err)
	}
	return nil
}

func (slot *SlotInfo) Lookup(key string) (any, bool) {
	return metautil.LookupAttr(slot.Attrs, nil, key)
}

func (slot *SlotInfo) Ref() SlotRef {
	return SlotRef{ProjectId: slot.Sdk.ProjectId, Workshop: slot.Sdk.Workshop, Sdk: slot.Sdk.Name, Name: slot.Name}
}

// SlotRef is a reference to a slot.
type SlotRef struct {
	ProjectId string `json:"project-id"`
	Workshop  string `json:"workshop"`
	Sdk       string `json:"sdk"`
	Name      string `json:"slot"`
}

func (ref SlotRef) SdkRef() Ref {
	return Ref{ProjectId: ref.ProjectId, Workshop: ref.Workshop, Sdk: ref.Sdk}
}

// String returns the "project-id/workshop/sdk:slot" representation of a slot reference.
func (ref SlotRef) String() string {
	return fmt.Sprintf("%s:%s", ref.SdkRef().String(), ref.Name)
}

// ShortRef returns the "workshop/sdk:slot" representation of a slot reference (human-friendly).
func (ref SlotRef) ShortRef() string {
	return fmt.Sprintf("%s:%s", ref.SdkRef().ShortRef(), ref.Name)
}

// SortsBefore returns true when slot should be sorted before the other
func (ref SlotRef) SortsBefore(other SlotRef) bool {
	if ref.Workshop != other.Workshop {
		return ref.Workshop < other.Workshop
	}
	if ref.Sdk != other.Sdk {
		return ref.Sdk < other.Sdk
	}
	return ref.Name < other.Name
}

// PlugInfo provides information about a plug.
type PlugInfo struct {
	Sdk *Info

	Name      string
	Interface string
	Attrs     map[string]any
	Label     string
}

func (plug *PlugInfo) Attr(key string, val any) error {
	v, ok := plug.Lookup(key)
	if !ok {
		ref := plug.Ref()
		return &AttributeNotFoundError{Attribute: key, Plug: &ref}
	}

	if err := metautil.SetValueFromAttribute(v, val); err != nil {
		return fmt.Errorf("invalid attribute %q for plug %q: %w", key, plug.Ref().ShortRef(), err)
	}
	return nil
}

func (plug *PlugInfo) Lookup(key string) (any, bool) {
	return metautil.LookupAttr(plug.Attrs, nil, key)
}

func (plug *PlugInfo) Ref() PlugRef {
	return PlugRef{ProjectId: plug.Sdk.ProjectId, Workshop: plug.Sdk.Workshop, Sdk: plug.Sdk.Name, Name: plug.Name}
}

// PlugRef is a reference to a plug.
type PlugRef struct {
	ProjectId string `json:"project-id"`
	Workshop  string `json:"workshop"`
	Sdk       string `json:"sdk"`
	Name      string `json:"plug"`
}

func (ref PlugRef) SdkRef() Ref {
	return Ref{ProjectId: ref.ProjectId, Workshop: ref.Workshop, Sdk: ref.Sdk}
}

// String returns the "project-id/workshop/sdk:plug" representation of a plug reference.
func (ref PlugRef) String() string {
	return fmt.Sprintf("%s:%s", ref.SdkRef().String(), ref.Name)
}

// ShortRef returns the "workshop/sdk:plug" representation of a plug reference (human-friendly).
func (ref PlugRef) ShortRef() string {
	return fmt.Sprintf("%s:%s", ref.SdkRef().ShortRef(), ref.Name)
}

// SortsBefore returns true when plug should be sorted before the other
func (ref PlugRef) SortsBefore(other PlugRef) bool {
	if ref.Workshop != other.Workshop {
		return ref.Workshop < other.Workshop
	}
	if ref.Sdk != other.Sdk {
		return ref.Sdk < other.Sdk
	}
	return ref.Name < other.Name
}

func SdkDir(sdkName string) string {
	return filepath.Join(dirs.WorkshopSdksDir, sdkName)
}

func SdkMetaPath(sdkName string) string {
	return filepath.Join(SdkDir(sdkName), "meta", "sdk.yaml")
}

func SdkHooksDir(sdkName string) string {
	return filepath.Join(SdkDir(sdkName), "sdk", "hooks")
}

func SdkHookPath(sdkName, hookName string) string {
	return filepath.Join(SdkHooksDir(sdkName), hookName)
}

func MockSanitizePlugsSlots(f func(sdkInfo *Info)) (restore func()) {
	old := SanitizePlugsSlots
	SanitizePlugsSlots = f
	return func() { SanitizePlugsSlots = old }
}

func MockInfo(c *check.C, yamlText string, projectId, workshop string) *Info {
	restoreSanitize := MockSanitizePlugsSlots(func(sdkInfo *Info) {})
	defer restoreSanitize()
	info, err := ReadSdkInfo([]byte(yamlText), projectId, workshop)
	c.Assert(err, check.IsNil)

	err = Validate(info)
	c.Assert(err, check.IsNil)
	return info
}

func MockInvalidInfo(c *check.C, yamlText string) *Info {
	restoreSanitize := MockSanitizePlugsSlots(func(sdkInfo *Info) {})
	defer restoreSanitize()

	sdkInfo, err := ReadSdkInfo([]byte(yamlText), "invalid", "ws")
	c.Assert(err, check.IsNil)
	err = Validate(sdkInfo)
	c.Assert(err, check.NotNil)
	return sdkInfo
}

// BadInterfacesSummary returns a summary of the problems of bad plugs
// and slots in the sdk.
func BadInterfacesSummary(sdkInfo *Info) string {
	inverted := make(map[string][]string)
	for name, reason := range sdkInfo.BadInterfaces {
		inverted[reason] = append(inverted[reason], name)
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%q SDK has bad plugs or slots: ", sdkInfo.Name)
	reasons := make([]string, 0, len(inverted))
	for reason := range inverted {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		names := inverted[reason]
		sort.Strings(names)
		for i, name := range names {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(name)
		}
		fmt.Fprintf(&buf, " (%s); ", reason)
	}
	return strings.TrimSuffix(buf.String(), "; ")
}
