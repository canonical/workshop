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

type snapshotCooldownSuite struct {
	cleanupCooldown string
	cleanupSet      bool
}

var _ = check.Suite(&snapshotCooldownSuite{})

func (s *snapshotCooldownSuite) SetUpTest(c *check.C) {
	s.cleanupCooldown, s.cleanupSet = os.LookupEnv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN")
	c.Assert(os.Unsetenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN"), check.IsNil)
}

func (s *snapshotCooldownSuite) TearDownTest(c *check.C) {
	if s.cleanupSet {
		c.Assert(os.Setenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN", s.cleanupCooldown), check.IsNil)
		return
	}
	c.Assert(os.Unsetenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN"), check.IsNil)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentDefaultsToOneHour(c *check.C) {
	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentUsesOverride(c *check.C) {
	c.Assert(os.Setenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN", "1s"), check.IsNil)
	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, time.Second)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentAllowsZeroDuration(c *check.C) {
	c.Assert(os.Setenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN", "0s"), check.IsNil)
	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, time.Duration(0))
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentIgnoresInvalidDuration(c *check.C) {
	c.Assert(os.Setenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN", "invalid"), check.IsNil)
	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}

func (s *snapshotCooldownSuite) TestSnapshotCooldownTimeFromEnvironmentIgnoresNegativeDuration(c *check.C) {
	c.Assert(os.Setenv("WORKSHOP_TEST_SNAPSHOT_CLEANUP_COOLDOWN", "-1s"), check.IsNil)
	c.Assert(snapshotCooldownTimeFromEnvironment(), check.Equals, defaultSnapshotCooldownTime)
}
