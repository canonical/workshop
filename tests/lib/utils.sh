#!/bin/bash

function setup_lxd() {
    if snap list lxd >/dev/null; then
        snap refresh --channel=6/stable lxd
    else
        snap install --channel=6/stable lxd
    fi
    lxd waitready --timeout=180

    # can already be initialised if reused
    # https://discuss.linuxcontainers.org/t/how-do-i-know-if-lxd-is-initialized/15473/3
    if [ "$(lxc storage list -f compact | grep -c default)" -eq 0 ]; then
        lxd init --auto --storage-backend=zfs
    fi

    # Import LXD base images instead of downloading in every spread instance.
    # This assumes images are mounted into the spread instance at /mnt.
    image_dir="/mnt"
    versions=("20.04" "22.04" "24.04")

    for version in "${versions[@]}"; do
        image_file="$image_dir/ubuntu-$version.tar.gz"
        image_root="$image_dir/ubuntu-$version.tar.gz.root"
        image_alias="workshop-ubuntu@$version-amd64"

        if [ -f "$image_file" ] && ! lxc image info "$image_alias" &>/dev/null; then
            lxc image import "$image_file" "$image_root" --alias "$image_alias"
        fi
    done
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

    while pgrep -f "apt|dpkg" >/dev/null; do
        echo "Waiting for any apt-related process to release the lock..."
        sleep 5
    done

    apt-get update
    apt-get install -y --no-install-recommends "linux-modules-extra-$(uname -r)" moreutils jq zfsutils-linux

    systemctl unmask snapd.service
    systemctl start snapd.service
    snap wait system seed.loaded
    # The /snap directory does not exist in some environments
    [ ! -d /snap ] && ln -s /var/lib/snapd/snap /snap

    setup_lxd

    snap install --classic --channel=1.24/stable go
    snap install yq
}

function setup_workshop() {
    snap install --dangerous --classic /workshop/tests/*.snap
    snap set workshop store.url=http://localhost:8080/storage/v1/
    snap set workshop workshop.image.server.url="$IMAGE_SERVER"
    snap restart workshop

    # required to keep /lib/systemd/systemd --user running for a regular user
    loginctl enable-linger ubuntu

    start_sdk_store
}

function cleanup_workshop() {
    stop_sdk_store

    snap remove workshop --purge
    snap remove sdkcraft --purge
    find /workshop -name .workshop.lock -delete
    loginctl disable-linger ubuntu
}

function start_sdk_store() {
    # run fake GCS bucket storage to emulate SDK store
    publish_test_sdks "$TESTS_SDKS" "$SDK_STORE_BUCKET_DIR"
    # a bug with fake-gcs-server that returns 404 if not owned by the user
    chown -R ubuntu.ubuntu /data
    mkdir -p /storage
    chown -R ubuntu.ubuntu /storage

    go install github.com/fsouza/fake-gcs-server@latest
    fake-gcs-server -data /data -scheme http -port 8080 -public-host localhost:8080 >~/fake_sdk_store.log 2>&1 &

    echo "Waiting for the fake SDK store to start on port 8080..."
    while ! nc -z localhost 8080; do
        sleep 2
    done
}

function stop_sdk_store() {
    pkill -f fake-gcs-server || true
    rm -rf /data
    rm -rf /storage
}

# Publish test SDKs in the fake SDK Store
function publish_test_sdks() {
    for i in "$1"/*; do
        SDK_NAME=$(basename "$i")
        SDK_FILE=$SDK_NAME.sdk
        SDK_PATH=$(readlink -f "$i")
        STORE_PATH="$2/$SDK_NAME"

        echo "$SDK_NAME:"
        find "$SDK_PATH/" -mindepth 2 -maxdepth 2 -type d -printf "%P\n" | while IFS= read -r track; do
            printf "\t%s -> %s\n" "$track" "$STORE_PATH/$track"
            mkdir -p "$STORE_PATH/$track"
            rm -f "$STORE_PATH/$track/$SDK_FILE"

            readarray -d '' -t SDK_FILES < <(ls --almost-all --zero "$SDK_PATH/$track")
            tar \
                --create \
                --format=posix \
                --use-compress-program='zstd -10 --threads=0' \
                --mode='a-st,go-w' \
                --owner='root:0' \
                --group='root:0' \
                --mtime="$(date --utc +'%Y-%m-%dT%H:%M:%S.%6NZ')" \
                --sort=name \
                --force-local \
                --file="$STORE_PATH/$track/$SDK_FILE" \
                --directory="$SDK_PATH/$track" \
                "${SDK_FILES[@]}"

            cp -f "$SDK_PATH/$track/meta/sdk.yaml" "$STORE_PATH/$track"
        done
    done
}

# Workshop sub-command wrappers
function workshop_exec() {
    sudo -u ubuntu -- workshop "$@" 2>&1
}

function run_sdkcraft() {
    sdkcraft "$@"
}

# Install sdkcraft from a local snap file
function install_sdkcraft() {
    if stat /sdkcraft/tests/*.snap 2>/dev/null; then
        snap install --classic --dangerous /sdkcraft/tests/*.snap
    else
        echo "Expected a snap to exist in /sdkcraft/tests/"
        exit 1
    fi
}
