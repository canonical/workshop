#!/usr/bin/env bash

set -eu
cd -- "$(dirname -- "$0")"

temp=$(mktemp --directory --tmpdir="$(pwd)")
trap 'rm -rf -- "$temp"' EXIT

cd "$temp"

# Limit to supported architectures for safety. Most architectures share the
# _IOWR macro in include/uapi/asm-generic/ioctl.h, but others have a different
# one in arch/<ARCH>/include/uapi/asm/ioctl.h. Currently I think FIFREEZE and
# related constants end up the same on all architectures regardless, but it's
# worth checking that when adding support for a new architecture.
cat <<EOF >zsysnum_linux.go
//go:build amd64 || arm64 || riscv64

EOF

ln -s ../sysnum_linux.go .
go tool cgo -godefs sysnum_linux.go >>zsysnum_linux.go

gofmt -w zsysnum_linux.go

mv zsysnum_linux.go ..
