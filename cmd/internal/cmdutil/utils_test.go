package cmdutil

import (
	"os"
	"path/filepath"

	"gopkg.in/check.v1"
)

type cmdUtils struct{}

var _ = check.Suite(&cmdUtils{})

func (m *cmdUtils) TestHomeDirectoryPathContraction(c *check.C) {
	home, _ := os.UserHomeDir()
	r := ContractHome(filepath.Join(home, "test"))
	c.Assert(r, check.Equals, "~/test")
	r = ContractHome(filepath.Join(home, "///test"))
	c.Assert(r, check.Equals, "~/test")
	r = ContractHome(home)
	c.Assert(r, check.Equals, "~")
	r = ContractHome("/sys")
	c.Assert(r, check.Equals, "/sys")
}
