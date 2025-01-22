package lxdbackend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/workshop"
)

func ProfileName(pid, workshop, sdk string) string {
	return strings.Join([]string{InstanceName(workshop, pid), sdk}, "-")
}

func DeviceConfigKey(sdk, dev string) string {
	return fmt.Sprintf("user.workshop.%s.%s", sdk, dev)
}

func DeviceTypeConfigKey(sdk, dev string) string {
	return fmt.Sprintf("user.workshop.%s.%s.type", sdk, dev)
}

func EnvironmentConfigKey(sdk string) string {
	return fmt.Sprintf("user.workshop.%s.environment", sdk)
}

func EnvironmentConfigValue(env []string) string {
	sb := strings.Builder{}
	for _, s := range env {
		sb.WriteString(s + ",")
	}
	return sb.String()
}

func EnvironmentConfigToSlice(env string) []string {
	var e []string
	splits := strings.Split(env, ",")
	e = append(e, splits...)
	return e
}

func Profile(conn lxd.InstanceServer, pid, wp, profile string) (workshop.SdkProfile, error) {
	name := ProfileName(pid, wp, profile)
	lxdp, _, err := conn.GetProfile(name)
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return workshop.SdkProfile{}, workshop.ErrSdkProfileNotFound
		}
		return workshop.SdkProfile{}, fmt.Errorf("cannot load %q profile (%w)", profile, err)
	}

	return lxdToSdkProfile(profile, lxdp.Devices, lxdp.Config)
}

func lxdToSdkProfile(profile string, devs map[string]map[string]string, config map[string]string) (workshop.SdkProfile, error) {
	var pr = workshop.NewSdkProfile(profile)
	pr.Environment = EnvironmentConfigToSlice(config[EnvironmentConfigKey(profile)])
	for name, dev := range devs {
		switch dev["type"] {
		case "disk":
			pr.Mounts[name] = workshop.Mount{Name: name, What: dev["source"], Where: dev["path"], Type: workshop.HostWorkshop}
		case "gpu":
			pr.Gpu = &workshop.Gpu{Name: name}
		case "proxy":
			devtype := config[DeviceTypeConfigKey(profile, name)]
			switch devtype {
			case "ssh-agent":
				pr.Agent = &workshop.SshAgent{ProxyEntry: workshop.ProxyEntry{Name: name, Connect: dev["connect"], Listen: dev["listen"]}}
			case "desktop-wayland":
				if pr.Desktop == nil {
					pr.Desktop = &workshop.Desktop{}
				}
				pr.Desktop.Wayland = &workshop.ProxyEntry{Name: name, Connect: dev["connect"], Listen: dev["listen"]}
			case "desktop-x11":
				if pr.Desktop == nil {
					pr.Desktop = &workshop.Desktop{}
				}
				pr.Desktop.X11 = &workshop.ProxyEntry{Name: name, Connect: dev["connect"], Listen: dev["listen"]}
			default:
				logger.Noticef("On reading %q SDK profile: unknown device type: %q", profile, devtype)
			}
		case "unix-char":
			devtype := config[DeviceTypeConfigKey(profile, name)]
			if devtype == "camera" {
				continue
			}

			logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, devtype)
		case "none":
			cfg, exist := config[DeviceConfigKey(profile, name)]
			if !exist {
				logger.Noticef("On reading %q SDK profile: unknown device %q", profile, name)
				continue
			}

			devtype := config[DeviceTypeConfigKey(profile, name)]
			switch devtype {
			case "camera":
				var camera workshop.Camera
				if err := json.Unmarshal([]byte(cfg), &camera); err != nil {
					return pr, err
				}
				pr.Camera = &camera
			case "mount":
				var mnt workshop.Mount
				if err := json.Unmarshal([]byte(cfg), &mnt); err != nil {
					return pr, err
				}
				pr.Mounts[name] = mnt
			default:
				logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, devtype)
			}
		default:
			logger.Noticef("On reading %q SDK profile: unknown device type %q", profile, dev["type"])
		}
	}
	return pr, nil
}
