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

package workshopstate

import (
	"os"
	"time"

	"gopkg.in/check.v1"
)

type snapshotCooldownSuite struct{}

var _ = check.Suite(&snapshotCooldownSuite{})

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentDefaultsToOneHour(c *check.C) {
	defer restoreEnv(testOverridesEnv)()
	defer restoreEnv(testSnapshotCleanupCooldownEnv)()

	c.Assert(os.Unsetenv(testOverridesEnv), check.IsNil)
	c.Assert(os.Unsetenv(testSnapshotCleanupCooldownEnv), check.IsNil)

	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentIgnoresCooldownWithoutTestOverride(c *check.C) {
	defer restoreEnv(testOverridesEnv)()
	defer restoreEnv(testSnapshotCleanupCooldownEnv)()

	c.Assert(os.Unsetenv(testOverridesEnv), check.IsNil)
	c.Assert(os.Setenv(testSnapshotCleanupCooldownEnv, "1s"), check.IsNil)

	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentUsesTestOverride(c *check.C) {
	defer restoreEnv(testOverridesEnv)()
	defer restoreEnv(testSnapshotCleanupCooldownEnv)()

	c.Assert(os.Setenv(testOverridesEnv, "1"), check.IsNil)
	c.Assert(os.Setenv(testSnapshotCleanupCooldownEnv, "1s"), check.IsNil)

	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, time.Second)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentAllowsZeroDuration(c *check.C) {
	defer restoreEnv(testOverridesEnv)()
	defer restoreEnv(testSnapshotCleanupCooldownEnv)()

	c.Assert(os.Setenv(testOverridesEnv, "1"), check.IsNil)
	c.Assert(os.Setenv(testSnapshotCleanupCooldownEnv, "0s"), check.IsNil)

	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, time.Duration(0))
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentIgnoresInvalidDuration(c *check.C) {
	defer restoreEnv(testOverridesEnv)()
	defer restoreEnv(testSnapshotCleanupCooldownEnv)()

	c.Assert(os.Setenv(testOverridesEnv, "1"), check.IsNil)
	c.Assert(os.Setenv(testSnapshotCleanupCooldownEnv, "invalid"), check.IsNil)

	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentIgnoresNegativeDuration(c *check.C) {
	defer restoreEnv(testOverridesEnv)()
	defer restoreEnv(testSnapshotCleanupCooldownEnv)()

	c.Assert(os.Setenv(testOverridesEnv, "1"), check.IsNil)
	c.Assert(os.Setenv(testSnapshotCleanupCooldownEnv, "-1s"), check.IsNil)

	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}

func restoreEnv(key string) func() {
	value, ok := os.LookupEnv(key)

	return func() {
		if ok {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	}
}
