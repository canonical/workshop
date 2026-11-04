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
	"path/filepath"
	"sync"
	"text/template"

	"github.com/canonical/x-go/strutil/shlex"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/workshop"
)

// cloudConfigVars holds the variables interpolated into the cloud-init
// user-data template.
type cloudConfigVars struct {
	// FsFreezePath is the path of the fsfreeze helper inside the workshop,
	// or empty for runtimes that do not freeze filesystems before
	// snapshots.
	FsFreezePath string

	// HasGRUB reports whether the runtime boots with GRUB and therefore
	// needs the machine ID set through the kernel command line.
	HasGRUB bool

	// StartTimeout is the wait-ready timeout in nanoseconds.
	StartTimeout int64

	// WorkshopCtlPath is the path of the workshopctl binary inside the
	// workshop.
	WorkshopCtlPath string

	// WorkshopSecretSocketPath is the path inside the workshop of the unix
	// socket that systemd units connect to for secret resolution.
	WorkshopSecretSocketPath string

	// WorkshopStateDir is the directory inside the workshop used to store
	// workshop state.
	WorkshopStateDir string
}

// cloudConfigTemplate returns the parsed cloud-init user-data template used
// when creating a workshop instance. The template is parsed once and reused.
var cloudConfigTemplate = sync.OnceValues(parseCloudConfigTemplate)

// makeCloudConfigVars constructs the variables for the cloud-init user-data
// template of the workshop described by file.
func makeCloudConfigVars(file *workshop.File) cloudConfigVars {
	var fsFreezePath string
	startTimeout := startTimeoutContainer
	if file.Runtime != workshop.RuntimeLXDContainer {
		fsFreezePath = dirs.FsFreezePath
		startTimeout = startTimeoutVM
	}
	return cloudConfigVars{
		FsFreezePath: fsFreezePath,
		HasGRUB:      file.Runtime == workshop.RuntimeLXDVM,
		StartTimeout: startTimeout.Nanoseconds(),
		WorkshopCtlPath: filepath.Join(
			dirs.WorkshopGuestBinDir,
			filepath.Base(dirs.WorkshopCtlPath),
		),
		WorkshopSecretSocketPath: dirs.WorkshopSecretSocketPath,
		WorkshopStateDir:         dirs.WorkshopStateDir,
	}
}

// parseCloudConfigTemplate parses the cloud-init user-data template.
func parseCloudConfigTemplate() (*template.Template, error) {
	const tmpl = `#cloud-config
users:
  - default
  - name: workshop
    primary_group: workshop
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    create_groups: false
    groups:
    - 'adm'
    - 'cdrom'
    - 'sudo'
    - 'dip'
    - 'plugdev'
    - 'audio'
    - 'netdev'
    - 'lxd'
    - 'video'
    - 'render'
    # Compatibility GIDs for various host systems:
    - '108' # netdev on 26.04
    - '111' # netdev on 24.04
    - '118' # netdev on 20.04
    - '119' # netdev on 22.04
    - '109' # render on 20.04
    - '110' # render on 22.04
    - '990' # render on 26.04
    - '992' # render on 24.04
bootcmd:
- |
  set -e
  maybe_groupadd() {
      # Ignore GID not unique (exit code 4) or group name not unique (exit code 9)
      groupadd -g "$1" -r "$2" || case $? in 4|9) ;; *) return $? ;; esac
  }
  maybe_groupadd 1000 workshop
  maybe_groupadd 108 netdev-compat-108
  maybe_groupadd 111 netdev-compat-111
  maybe_groupadd 118 netdev-compat-118
  maybe_groupadd 119 netdev-compat-119
  maybe_groupadd 109 render-compat-109
  maybe_groupadd 110 render-compat-110
  maybe_groupadd 990 render-compat-990
  maybe_groupadd 992 render-compat-992
- chmod 0600 /etc/ssh/ssh_host_ed25519_key
# LXD containers use /dev/urandom from the host and VMs use virtio-rng.
- install -D --mode=0644 /dev/null /var/cache/pollinate/seeded
apt:
  conf: |
    # Installed by workshop

    # Don't automatically install recommended packages
    APT::Install-Recommends "0";

    # Don't automatically install suggested packages
    APT::Install-Suggests "0";

    # Bypass confirmation prompts
    APT::Get::Assume-Yes "1";
grub_dpkg:
  enabled: false
ssh_deletekeys: false
ssh_genkeytypes: [ed25519]
write_files:
  - path: /etc/cloud/cloud-init.disabled
    defer: true
  # Workaround https://bugs.launchpad.net/snapd/+bug/2165972. Without network
  # access snapd retries certain HTTP requests a few times before timing out.
  - path: /etc/systemd/system/snapd.service.d/70-snapd-after-network.conf
    content: |
      [Unit]
      After=network.target
  - path: /etc/ssh/sshd_config.d/90-workshop.conf
    content: |
      HostCertificate /etc/ssh/ssh_host_ed25519_key-cert.pub
      TrustedUserCAKeys /etc/ssh/ssh_ca_ed25519_key.pub
  # Workaround https://github.com/canonical/lxd/issues/19055. We handle the
  # initial permissions in bootcmd, but subsequent boots from a VM snapshot
  # can reset the permissions back to 0644.
  - path: /etc/systemd/system/ssh.service.d/70-workshop.conf
    content: |
      [Service]
      ExecStartPre=
      ExecStartPre=/usr/bin/chmod 0600 /etc/ssh/ssh_host_ed25519_key
      ExecStartPre=/usr/sbin/sshd -t
  - path: /etc/systemd/system/workshop-waitready.service
    content: |
      [Unit]
      Description=Signal workshop readiness to LXD
      After=dbus.socket
      Requires=dbus.socket

      [Service]
      Type=notify
      ExecStart=/usr/local/lib/workshop/waitready
      Environment=WORKSHOP_WAITREADY_TIMEOUT_NS={{.StartTimeout}}
      Restart=on-failure
      RestartSec=2s

      [Install]
      WantedBy=multi-user.target
{{- if .HasGRUB}}
  - path: /boot/grub/custom.cfg
    content: |
      # Extract SMBIOS UUID and store it in a GRUB variable. We use it to set
      # the systemd.machine_id kernel parameter to the LXD UUID, which forces
      # systemd to use it. By default it prefers reading the machine ID from
      # /etc/machine-id, which may be stale when restoring from a snapshot.
      insmod smbios
      smbios --type 1 --get-uuid 8 --set workshop_machine_id
      export workshop_machine_id
  - path: /etc/default/grub.d/70-workshop.cfg
    content: |
      GRUB_CMDLINE_LINUX="${GRUB_CMDLINE_LINUX:+$GRUB_CMDLINE_LINUX }"'systemd.machine_id=${workshop_machine_id}'
      GRUB_CMDLINE_LINUX_DEFAULT="${GRUB_CMDLINE_LINUX_DEFAULT:+$GRUB_CMDLINE_LINUX_DEFAULT }quiet"
{{- end}}
  - path: /etc/systemd/system/workshop-secret.socket
    content: |
      [Unit]
      Description=Workshop systemd load credential socket

      [Socket]
      ListenStream={{.WorkshopSecretSocketPath}}
      SocketMode=0660
      Accept=yes

      [Install]
      WantedBy=sockets.target
  - path: /etc/systemd/system/workshop-secret@.service
    content: |
      [Unit]
      Description=Workshop systemd load credential socket resolver

      [Service]
      ExecStart={{.WorkshopCtlPath}} get-secret --systemd
      StandardError=journal
      StandardInput=socket
      StandardOutput=socket
runcmd:
  # Project directory is required for 'workshop exec'.
  - install --directory --mode=755 /project /usr/local/bin /usr/local/lib/workshop {{shquote .WorkshopStateDir}}
  # Create XDG base directories so SDKs don't need an extra mode=700 step.
  - install --directory --mode=700 --owner=workshop --group=workshop /home/workshop/.cache /home/workshop/.config /home/workshop/.local
  # Create ~/.local/bin so SDKs don't need to source ~/.profile to add it to the PATH.
  - install --directory --mode=755 --owner=workshop --group=workshop /home/workshop/.local/bin
  # Put workshopctl on the PATH.
  - ln -sf {{shquote .WorkshopCtlPath}} /usr/local/bin/workshopctl
  - ln -sf ../../bin/workshopctl /usr/local/lib/workshop/waitready
{{- if ne .FsFreezePath ""}}
  - ln -sf ../../bin/workshopctl {{shquote .FsFreezePath}}
{{- end}}
  - systemctl enable --now workshop-waitready.service
  - systemctl enable --now workshop-secret.socket
  # Linger starts the user manager for the specified user on boot, which then creates /run/user/$UID,
  # sets $XDG_RUNTIME_DIR and more. Interfaces such as desktop rely on both of these to be present.
  # This does not introduce any additional modification beyond what a login session would normally create.
  - loginctl enable-linger workshop
{{- if .HasGRUB}}
  - update-grub
{{- end}}
`

	funcs := map[string]any{
		"shquote": shlex.Quote,
	}

	return template.New("cloud-config").Funcs(funcs).Parse(tmpl)
}
