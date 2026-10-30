SHELL := /bin/bash
.SHELLFLAGS := -o errexit -o pipefail -c
.ONESHELL:
.NOTPARALLEL:

ROOT := $(CURDIR)
LIBKRUN_REV := 6254f5e22b8782a4dffe530ff44f1bbe623332e0
LXD_REPO ?= git@github.com:jonathan-conder/lxd.git
LXD_BRANCH ?= feature/microvm

.PHONY: all clone libkrun libkrunfw lxd
all: clone libkrun libkrunfw lxd

clone:
	if [ ! -d "$(ROOT)/libkrun/.git" ]; then
		git clone --filter=tree:0 --no-checkout https://github.com/libkrun/libkrun.git "$(ROOT)/libkrun"
		git -C "$(ROOT)/libkrun" fetch --filter=tree:0 origin "$(LIBKRUN_REV)"
		git -C "$(ROOT)/libkrun" checkout --quiet --detach FETCH_HEAD
	fi

	# The pinned libkrun console device aborts if the guest re-opens an already
	# active port (e.g. a getty restart). Patch it to ignore the duplicate open.
	patch="$(ROOT)/.workshop/patches/libkrun-console-reopen.patch"
	if git -C "$(ROOT)/libkrun" apply --check "$$patch" >/dev/null 2>&1; then
		git -C "$(ROOT)/libkrun" apply "$$patch"
	elif ! git -C "$(ROOT)/libkrun" apply --reverse --check "$$patch" >/dev/null 2>&1; then
		echo "error: libkrun console guard does not apply cleanly" >&2
		exit 1
	fi

	if [ ! -d "$(ROOT)/libkrunfw/.git" ]; then
		git clone --filter=tree:0 https://github.com/libkrun/libkrunfw.git "$(ROOT)/libkrunfw"
	fi

	if [ ! -d "$(ROOT)/lxd/.git" ]; then
		git clone --filter=tree:0 --branch "$(LXD_BRANCH)" "$(LXD_REPO)" "$(ROOT)/lxd"
	fi

libkrun: clone
	$(MAKE) -C $(ROOT)/libkrun BLK=1 NET=1 -j$$(nproc)

libkrunfw: clone
	$(MAKE) -C $(ROOT)/libkrunfw MAKEFLAGS=-j$$(nproc)

lxd: clone
	$(MAKE) -C $(ROOT)/lxd deps
	cd $(ROOT)/lxd
	eval "$$($(MAKE) -s env)"
	mkdir -p $(ROOT)/.microvm/bin
	GOBIN=$(ROOT)/.microvm/bin $(MAKE) -j$$(nproc) lxd client
