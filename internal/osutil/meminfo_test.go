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

package osutil_test

import (
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
)

type meminfoSuite struct{}

var _ = Suite(&meminfoSuite{})

const meminfoExampleFromLiveSystem = `MemTotal:       32876680 kB
MemFree:         3478104 kB
MemAvailable:   20527364 kB
Buffers:         1584432 kB
Cached:         14550292 kB
SwapCached:            0 kB
Active:          8344864 kB
Inactive:       16209412 kB
Unevictable:        2152 kB
Mlocked:            2152 kB
SwapTotal:             0 kB
SwapFree:              0 kB
CmaTotal:              0 kB
CmaFree:               0 kB
`

const meminfoExampleFromPi3 = `MemTotal:         929956 kB
MemFree:           83420 kB
MemAvailable:     676936 kB
Buffers:           84036 kB
Cached:           439980 kB
SwapCached:            0 kB
SwapTotal:             0 kB
SwapFree:              0 kB
CmaTotal:         131072 kB
CmaFree:            8664 kB
`

func (s *meminfoSuite) TestMemInfoHappy(c *C) {
	p := filepath.Join(c.MkDir(), "meminfo")
	restore := osutil.MockProcMeminfo(p)
	defer restore()

	c.Assert(os.WriteFile(p, []byte(meminfoExampleFromLiveSystem), 0644), IsNil)

	mi, err := osutil.ReadMemInfo()
	c.Assert(err, IsNil)
	c.Check(mi.Total, Equals, uint64(32876680)*1024)
	c.Check(mi.Available, Equals, uint64(20527364)*1024)
	c.Check(mi.SwapTotal, Equals, uint64(0))

	c.Assert(os.WriteFile(p, []byte("MemTotal:    1234 kB\nMemAvailable:    567 kB\n"), 0644), IsNil)

	mi, err = osutil.ReadMemInfo()
	c.Assert(err, IsNil)
	c.Check(mi.Total, Equals, uint64(1234)*1024)
	c.Check(mi.Available, Equals, uint64(567)*1024)

	const meminfoReorderedWithEmptyLine = `MemAvailable:   20527370 kB

MemTotal:       32876699 kB
MemFree:         3478104 kB
`
	c.Assert(os.WriteFile(p, []byte(meminfoReorderedWithEmptyLine), 0644), IsNil)

	mi, err = osutil.ReadMemInfo()
	c.Assert(err, IsNil)
	c.Check(mi.Total, Equals, uint64(32876699)*1024)
	c.Check(mi.Available, Equals, uint64(20527370)*1024)

	c.Assert(os.WriteFile(p, []byte(meminfoExampleFromPi3), 0644), IsNil)

	// CmaTotal is taken correctly into account
	mi, err = osutil.ReadMemInfo()
	c.Assert(err, IsNil)
	c.Check(mi.Total, Equals, uint64(929956-131072)*1024)
	c.Check(mi.Available, Equals, uint64(676936)*1024)
}

func (s *meminfoSuite) TestMemInfoFromHost(c *C) {
	mi, err := osutil.ReadMemInfo()
	c.Assert(err, IsNil)
	c.Check(mi.Total > uint64(32*1024*1024),
		Equals, true, Commentf("unexpected system memory %v", mi.Total))
	c.Check(mi.Available <= mi.Total,
		Equals, true, Commentf("unexpected available memory %v", mi.Available))
}

func (s *meminfoSuite) TestMemInfoUnhappy(c *C) {
	p := filepath.Join(c.MkDir(), "meminfo")
	restore := osutil.MockProcMeminfo(p)
	defer restore()

	const noTotalMem = `MemFree:         3478104 kB
MemAvailable:   20527364 kB
Buffers:         1584432 kB
Cached:         14550292 kB
`
	const notkBTotalMem = `MemTotal:         3478104 MB
`
	const missingFieldsTotalMem = `MemTotal:  1234
`
	const badTotalMem = `MemTotal:  abcdef kB
`
	const hexTotalMem = `MemTotal:  0xabcdef kB
`
	const noAvailableMem = `MemTotal:        32876680 kB
MemFree:         3478104 kB
`

	for _, tc := range []struct {
		content, err string
	}{
		{
			content: noTotalMem,
			err:     `cannot determine the total amount of memory in the system from .*/meminfo`,
		}, {
			content: notkBTotalMem,
			err:     `cannot process unexpected meminfo entry "MemTotal:         3478104 MB"`,
		}, {
			content: missingFieldsTotalMem,
			err:     `cannot process unexpected meminfo entry "MemTotal:  1234"`,
		}, {
			content: badTotalMem,
			err:     `cannot convert memory size value: strconv.ParseUint: parsing "abcdef": invalid syntax`,
		}, {
			content: hexTotalMem,
			err:     `cannot convert memory size value: strconv.ParseUint: parsing "0xabcdef": invalid syntax`,
		}, {
			content: noAvailableMem,
			err:     `cannot determine the available amount of memory in the system from .*/meminfo`,
		},
	} {
		c.Assert(os.WriteFile(p, []byte(tc.content), 0644), IsNil)
		mi, err := osutil.ReadMemInfo()
		c.Assert(err, ErrorMatches, tc.err)
		c.Check(mi, IsNil)
	}
}
