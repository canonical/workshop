#!/bin/bash

# shellcheck source=tests/lib/retry.sh
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/retry.sh"

function setup_lxd() {
    if ! snap list lxd >/dev/null; then
        retry 5 snap install --channel="$LXD_CHANNEL" lxd
    elif [ -z "${LXD_CUSTOM_SNAP}" ]; then
        retry 5 snap refresh --channel="$LXD_CHANNEL" lxd
    fi
    if [ -n "${LXD_CUSTOM_SNAP}" ]; then
        (cd -- "$PROJECT_PATH" && snap install --dangerous "${LXD_CUSTOM_SNAP}")
        snap alias lxd.lxc lxc
    fi
    lxd waitready --timeout=180

    # can already be initialised if reused or if a non-default pool exists
    # https://discuss.linuxcontainers.org/t/how-do-i-know-if-lxd-is-initialized/15473/3
    if [ "$(lxc storage list --format=csv | wc -l)" -eq 0 ]; then
        lxd init --auto --storage-backend=zfs
    fi
}

function prepare_environment() {
    # The unattended upgrades hold locks on reusable instances and can break a
    # spread run. This is to prevent the prepare script from failing (e.g. when
    # reusing an existing spread instance). Since workshops don't currently
    # interact with apt/dpkg on the host, it shouldn't have implications for the
    # tests.
    systemctl stop unattended-upgrades.service || true
    systemctl disable unattended-upgrades.service || true
    systemctl disable apt-daily.timer || true
    systemctl disable apt-daily-upgrade.timer || true

    # Configure apt to retry operations to handle flaky network issues
    # This helps with mirror sync errors and transient network failures
    cat <<EOF >/etc/apt/apt.conf.d/80-retries
Acquire::Retries "10";
Acquire::http::Timeout "30";
Acquire::http::Pipeline-Depth "0";
Acquire::CompressionTypes::Order { "gz"; "xz"; };
EOF

    while pgrep -f "apt|dpkg" >/dev/null; do
        echo "Waiting for any apt-related process to release the lock..."
        sleep 5
    done

    pkgs=(
        bsdutils
        fish
        jq
        "linux-modules-extra-$(uname -r)"
        moreutils
        python3-sdnotify
        python3-uinput
        zfsutils-linux
        zsh
    )
    retry 5 apt-get update
    apt-get install -y --no-install-recommends "${pkgs[@]}"

    mkdir -p /etc/systemd/system/snapd.service.d
    cat <<EOF >/etc/systemd/system/snapd.service.d/override.conf
# Workaround for https://bugs.launchpad.net/snapd/+bug/2104066
[Service]
Environment=SNAPD_STANDBY_WAIT=1m
EOF
    systemctl daemon-reload

    systemctl unmask snapd.service
    systemctl restart snapd.service
    snap wait system seed.loaded
    # The /snap directory does not exist in some environments
    [ ! -d /snap ] && ln -s /var/lib/snapd/snap /snap

    setup_lxd

    retry 5 snap install --classic --channel=1.26/stable go
    retry 5 snap install yq

    chown -R ubuntu:ubuntu /workshop
}

function setup_workshop() {
    snap install --dangerous --classic /workshop/tests/*.snap

    snap set workshop workshop.debug=1
    snap set workshop workshop.experimental-vms=1
    snap set workshop workshop.image.server.url="$IMAGE_SERVER"
    snap alias workshop.sdk sdk
    snap restart workshop

    # required to keep /lib/systemd/systemd --user running for a regular user
    loginctl enable-linger ubuntu
}

function cleanup_workshop() {
    snap remove workshop --purge
    snap remove sdkcraft --purge
    find /workshop -name .workshop.lock -delete
    loginctl disable-linger ubuntu
}

# Workshop sub-command wrappers
function workshop_exec() {
    sudo -u ubuntu -- workshop "$@" 2>&1
}

function sdk_exec() {
    sudo -u ubuntu -- sdk "$@" 2>&1
}

function run_sdkcraft() {
    sudo -u ubuntu -- sdkcraft "$@"
}

function install_sdkcraft() {
    snap install --classic --edge sdkcraft
}
