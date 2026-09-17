// Copyright (c) 2021 Canonical Ltd
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

package osutil

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var (
	procMeminfo = "/proc/meminfo"
)

// MemInfo holds the parts of /proc/meminfo we care about, in bytes.
type MemInfo struct {
	// Total is the total usable memory in the system.
	//
	// Usable means (MemTotal - CmaTotal), i.e. the total amount of memory
	// minus the space reserved for the CMA (Contiguous Memory Allocator).
	//
	// CMA memory is taken up by e.g. the framebuffer on the Raspberry Pi or
	// by DSPs on specific boards.
	Total uint64

	// Available is the kernel's own estimate of how much memory can be
	// handed out without pushing the system into swapping or reclaim.
	Available uint64

	// SwapTotal is the configured swap size. Without swap, running out of
	// memory kills processes rather than slowing the system down.
	SwapTotal uint64
}

// ReadMemInfo returns the memory accounting reported by the kernel.
func ReadMemInfo() (*MemInfo, error) {
	f, err := os.Open(procMeminfo)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)

	var memTotal, cmaTotal, memAvailable, swapTotal uint64
	for s.Scan() {
		var p *uint64
		l := strings.TrimSpace(s.Text())
		switch {
		case strings.HasPrefix(l, "MemTotal:"):
			p = &memTotal
		case strings.HasPrefix(l, "CmaTotal:"):
			p = &cmaTotal
		case strings.HasPrefix(l, "MemAvailable:"):
			p = &memAvailable
		case strings.HasPrefix(l, "SwapTotal:"):
			p = &swapTotal
		default:
			continue
		}
		fields := strings.Fields(l)
		if len(fields) != 3 || fields[2] != "kB" {
			return nil, fmt.Errorf("cannot process unexpected meminfo entry %q", l)
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert memory size value: %v", err)
		}
		*p = v * 1024
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if memTotal == 0 {
		return nil, fmt.Errorf("cannot determine the total amount of memory in the system from %s", procMeminfo)
	}
	if memAvailable == 0 {
		return nil, fmt.Errorf("cannot determine the available amount of memory in the system from %s", procMeminfo)
	}
	return &MemInfo{
		Total:     memTotal - cmaTotal,
		Available: memAvailable,
		SwapTotal: swapTotal,
	}, nil
}

func MockProcMeminfo(newPath string) (restore func()) {
	MustBeTestBinary("mocking can only be done from tests")
	oldProcMeminfo := procMeminfo
	procMeminfo = newPath
	return func() {
		procMeminfo = oldProcMeminfo
	}
}
