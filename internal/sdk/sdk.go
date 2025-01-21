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
)

type Setup struct {
	Name             string     `json:"name"`
	Channel          string     `json:"channel"`
	Revision         Revision   `json:"revision"`
	RevisionSequence []Revision `json:"revision-sequence,omitempty"`
	InstallTime      *time.Time `json:"install-time"`
}

func (s *Setup) Filepath() string {
	return filepath.Join(dirs.SdkDownloads, s.Filename())
}

func (s *Setup) Filename() string {
	return fmt.Sprintf("%s_%s.sdk", s.Name, s.Revision.String())
}

func VolumeName(name, revision string) string {
	return fmt.Sprintf("%s-%s", name, revision)
}

type sdkYaml struct {
	Name      string                 `yaml:"name"`
	Base      string                 `yaml:"base"`
	Version   string                 `yaml:"version,omitempty"`
	Type      string                 `yaml:"type"`
	BuildTime *time.Time             `yaml:"sdkcraft-started-at,omitempty"`
	Plugs     map[string]interface{} `yaml:"plugs,omitempty"`
	Slots     map[string]interface{} `yaml:"slots,omitempty"`
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

type Info struct {
	ProjectId string
	Workshop  string
	Name      string
	Base      string
	Version   string
	Type      Type
	Revision  Revision
	Channel   string
	BuildTime *time.Time

	Plugs     map[string]*PlugInfo
	PlugBinds map[string]*PlugBind
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

func (i *Info) SetupPlugBinds(binds map[string]*PlugBind) error {
	if i.Type == System {
		return nil
	}

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
func (i *Info) SetupWorkshopSlots(slots map[string]interface{}) error {
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
func (i *Info) SetupWorkshopPlugs(plugs map[string]interface{}) error {
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
		return &Info{}, err
	}

	if sdkYaml.Type == "" {
		sdkYaml.Type = Regular.String()
	}

	sdkInfo := &Info{
		ProjectId:     projectId,
		Workshop:      workshop,
		Name:          sdkYaml.Name,
		Base:          sdkYaml.Base,
		Version:       sdkYaml.Version,
		Type:          Type(sdkYaml.Type),
		BuildTime:     sdkYaml.BuildTime,
		Plugs:         make(map[string]*PlugInfo),
		PlugBinds:     make(map[string]*PlugBind),
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

func convertToSlotOrPlugData(plugOrSlot, name string, data interface{}) (iface, label string, attrs map[string]interface{}, err error) {
	iface = name
	switch data := data.(type) {
	case string:
		return data, "", nil, nil
	case nil:
		return name, "", nil, nil
	case map[string]interface{}:
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
					attrs = make(map[string]interface{})
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
	Attrs     map[string]interface{}
	Label     string
}

type AttributeNotFoundError struct{ Err error }

func (e AttributeNotFoundError) Error() string {
	return e.Err.Error()
}

func (e AttributeNotFoundError) Is(target error) bool {
	_, ok := target.(AttributeNotFoundError)
	return ok
}

func lookupAttr(attrs map[string]interface{}, path string) (interface{}, bool) {
	var v interface{}
	comps := strings.FieldsFunc(path, func(r rune) bool { return r == '.' })
	if len(comps) == 0 {
		return nil, false
	}
	v = attrs
	for _, comp := range comps {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		v, ok = m[comp]
		if !ok {
			return nil, false
		}
	}

	return v, true
}

func getAttribute(sdkName string, ifaceName string, attrs map[string]interface{}, key string, val interface{}) error {
	v, ok := lookupAttr(attrs, key)
	if !ok {
		return AttributeNotFoundError{fmt.Errorf("SDK %q does not have attribute %q for interface %q", sdkName, key, ifaceName)}
	}

	return metautil.SetValueFromAttribute(sdkName, ifaceName, key, v, val)
}

func (slot *SlotInfo) Attr(key string, val interface{}) error {
	return getAttribute(slot.Sdk.Name, slot.Interface, slot.Attrs, key, val)
}

func (slot *SlotInfo) Lookup(key string) (interface{}, bool) {
	return lookupAttr(slot.Attrs, key)
}

// String returns the representation of the slot as sdk:slot string.
func (slot *SlotInfo) String() string {
	return fmt.Sprintf("%s:%s", slot.Sdk.Name, slot.Name)
}

// PlugInfo provides information about a plug.
type PlugInfo struct {
	Sdk *Info

	Name      string
	Interface string
	Attrs     map[string]interface{}
	Label     string
}

func (plug *PlugInfo) Attr(key string, val interface{}) error {
	return getAttribute(plug.Sdk.Name, plug.Interface, plug.Attrs, key, val)
}

func (plug *PlugInfo) Lookup(key string) (interface{}, bool) {
	return lookupAttr(plug.Attrs, key)
}

type PlugBind struct {
	ProjectId string
	Workshop  string
	Sdk       string
	Name      string
}

func SdkRootPath(sdkName string) string {
	return filepath.Join(dirs.WorkshopSdksDir, sdkName)
}

func SdkRevPath(sdkName string, rev string) string {
	return filepath.Join(SdkRootPath(sdkName), rev)
}

func SdkCurrentPath(sdkName string) string {
	return filepath.Join(SdkRootPath(sdkName), "current")
}

func SdkMetaDir(sdkName string) string {
	return filepath.Join(SdkCurrentPath(sdkName), "meta")
}

func SdkMetaPath(sdkName string) string {
	return filepath.Join(SdkMetaDir(sdkName), "sdk.yaml")
}

func SdkHooksDir(sdkName string) string {
	return filepath.Join(SdkCurrentPath(sdkName), "sdk", "hooks")
}

func SdkHookPath(sdkName, hookName string) string {
	return filepath.Join(SdkHooksDir(sdkName), hookName)
}

func ProjectUserData(homedir, pid string) string {
	return filepath.Join(homedir, ".local", "share", "workshop", "project", pid)
}

func ProjectContentDir(homedir, pid string) string {
	return filepath.Join(ProjectUserData(homedir, pid), "mount")
}

func ProjectSketchSdkDir(homedir, pid string) string {
	return filepath.Join(ProjectUserData(homedir, pid), "sdk", "sketch")
}

func WorkshopSketchSdk(homedir, pid, wp string) string {
	return filepath.Join(ProjectSketchSdkDir(homedir, pid), wp)
}

func WorkshopSketchSdkCurrent(homedir, pid, wp string) string {
	return filepath.Join(ProjectSketchSdkDir(homedir, pid), wp, "current")
}

func WorkshopSketchSdkStash(homedir, pid, wp string) string {
	return filepath.Join(ProjectSketchSdkDir(homedir, pid), wp, "stash")
}

func SdkMountHostSource(homedir, pid, wp, sdk, plug string) string {
	dir := strings.Join([]string{wp, sdk, plug}, "_") + ".sdk"
	return filepath.Join(ProjectContentDir(homedir, pid), dir)
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
