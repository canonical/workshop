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

package daemon

import (
	"net/http"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/version"
)

func (s *apiSuite) TestSystemInfoOk(c *check.C) {
	s.daemon(c)
	restore := version.MockVersion("1.2.3")
	defer restore()

	req, err := http.NewRequest("GET", "/v1/system-info", nil)
	c.Assert(err, check.IsNil)

	rsp := v1GetSystemInfo(apiCmd("/v1/system-info"), req, nil).(*resp)
	c.Check(rsp.Type, check.Equals, ResponseTypeSync)
	c.Check(rsp.Status, check.Equals, http.StatusOK)
	c.Check(rsp.Result, check.DeepEquals, &sysInfo{Version: "1.2.3"})
}
