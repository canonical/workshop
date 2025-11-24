// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package ifacestate_test

import (
	"testing"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/overlord/ifacestate"
	"github.com/canonical/workshop/internal/overlord/ifacestate/schema"
	"github.com/canonical/workshop/internal/overlord/state"
	"github.com/canonical/workshop/internal/sdk"
)

type helpersSuite struct {
	st *state.State
}

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&helpersSuite{})

func (s *helpersSuite) SetUpTest(c *check.C) {
	s.st = state.New(nil)
}

var workshopTemplate = `name: %s
base: ubuntu@20.04
sdks:
  {{- range . }}
  - name: {{ .Name}}
    channel: {{.Channel}}
  {{- end }}
`

var consumerYaml = `name: consumer
base: ubuntu@22.04
plugs:
  plug-mount:
    interface: mount
    workshop-target: /project/mount
`

var producerYaml = `name: producer
base: ubuntu@22.04
slots:
  slot-mount:
    interface: mount
    workshop-source: $SDK/produce
`

func (s *helpersSuite) TestAutoConnectChecker(c *check.C) {
	consumer := sdk.MockInfo(c, consumerYaml, "42424242", "ws")
	plugMount := interfaces.NewConnectedPlug(consumer.Plugs["plug-mount"], nil, nil)

	system := sdk.MockInfo(c, systemYaml, "42424242", "ws")
	autoMount := interfaces.NewConnectedSlot(system.Slots["mount"], nil, nil)

	producer := sdk.MockInfo(c, producerYaml, "42424242", "ws")
	slotMount := interfaces.NewConnectedSlot(producer.Slots["slot-mount"], nil, nil)

	workshopConns := []interfaces.ConnRef{}
	policyCheck := ifacestate.AutoConnectChecker(workshopConns)

	// slots named "mount" can be auto-connected
	ok, err := policyCheck(plugMount, autoMount)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)

	// other slots can be auto-connected via the workshop definition
	_, err = policyCheck(plugMount, slotMount)
	c.Assert(err, check.NotNil)

	connRef := interfaces.ConnRef{PlugRef: plugMount.Ref(), SlotRef: slotMount.Ref()}
	workshopConns = append(workshopConns, connRef)
	policyCheck = ifacestate.AutoConnectChecker(workshopConns)

	ok, err = policyCheck(plugMount, slotMount)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
}

func (s *helpersSuite) TestGetConns(c *check.C) {
	s.st.Lock()
	defer s.st.Unlock()
	s.st.Set("conns", map[string]interface{}{
		"42424242/ws/app:mount 42424242/ws/core:mount": map[string]interface{}{
			"auto":      true,
			"interface": "mount",
			"slot-static": map[string]interface{}{
				"number": int(78),
			},
		},
	})

	conns, err := ifacestate.GetConns(s.st)
	c.Assert(err, check.IsNil)
	for id, connState := range conns {
		c.Assert(id, check.Equals, "42424242/ws/app:mount 42424242/ws/core:mount")
		c.Assert(connState.Auto, check.Equals, true)
		c.Assert(connState.Interface, check.Equals, "mount")
		c.Assert(connState.StaticSlotAttrs["number"], check.Equals, int64(78))
	}
}

func (s *helpersSuite) TestSetConns(c *check.C) {
	s.st.Lock()
	defer s.st.Unlock()

	conns := map[string]*schema.ConnState{
		"42424242/ws/app:mount 42424242/ws/core:mount": {Auto: true, Interface: "mount"},
	}

	ifacestate.SetConns(s.st, conns)
	var readconns map[string]interface{}
	err := s.st.Get("conns", &readconns)
	c.Assert(err, check.IsNil)
	c.Assert(readconns, check.DeepEquals, map[string]interface{}{
		"42424242/ws/app:mount 42424242/ws/core:mount": map[string]interface{}{
			"auto":      true,
			"interface": "mount",
		},
	})
}
