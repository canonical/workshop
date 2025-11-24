// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2024 Canonical Ltd
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

package osutil_test

import (
	"crypto/sha1"
	"crypto/sha3"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path/filepath"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/testutil"
)

// Embed some files from the initial Workshop commit, so we can check our
// directory hash works the same as git. Only works because we use 644
// permissions. Subdirectories won't work because git lists them as 000.
//
// $ git rev-list --max-parents=0 HEAD
// c1e044fdaf453800f8eca00abe10ae8941ba5c1b
// $ git rev-parse c1e044fdaf453800f8eca00abe10ae8941ba5c1b:cmd
// 54b847f650bbb2485acf1101facf009abd4de2b4
// $ git cat-file -p 54b847f650bbb2485acf1101facf009abd4de2b4
// 100644 blob 490aed778fb6c9d8d63ae30060dda337f815534d	launch.go
// 100644 blob bd3b483ffe002e94a3339c2ac980e4f6994b996d	main.go
const (
	launchGo = `package main

import (
	workspace "github.com/canonical/workspace/internal"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

type CmdLaunch struct {
}

func (c *CmdLaunch) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "launch [workspace-name]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Launch a workspace",
		RunE:  c.Run,
	}

	return cmd
}

func (c *CmdLaunch) Run(cmd *cobra.Command, av []string) error {
	fs := afero.NewOsFs()
	_, err := workspace.NewWorkspace(fs, ".workspace.project.yaml")
	return err
}
`

	mainGo = `package main

import (
	"github.com/spf13/cobra"
)

func main() {
	app := &cobra.Command{
		Use:           "workspace",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	app.AddCommand((&CmdLaunch{}).Command())

	if err := app.Execute(); err != nil {
		return
	}
}
`
)

type hashSuite struct {
	testutil.BaseTest
}

var _ = check.Suite(&hashSuite{})

func (s *hashSuite) TestHashDirEntriesMatchesGit(c *check.C) {
	root := c.MkDir()

	c.Assert(os.WriteFile(filepath.Join(root, "launch.go"), []byte(launchGo), 0o644), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "main.go"), []byte(mainGo), 0o644), check.IsNil)

	digest, err := osutil.HashDirEntries(sha1.New().(hash.Cloner), root)
	c.Assert(err, check.IsNil)
	c.Check(hex.EncodeToString(digest), check.Equals, "54b847f650bbb2485acf1101facf009abd4de2b4")
}

func (s *hashSuite) TestHashDirEntriesRespectsPerms(c *check.C) {
	root := c.MkDir()

	c.Assert(os.Mkdir(filepath.Join(root, "dir"), 0o755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "file"), []byte("foo"), 0o644), check.IsNil)

	digest, err := osutil.HashDirEntries(sha3.New384(), root)
	c.Assert(err, check.IsNil)
	first := hex.EncodeToString(digest)

	c.Assert(os.Chmod(filepath.Join(root, "dir"), 0o775), check.IsNil)

	digest, err = osutil.HashDirEntries(sha3.New384(), root)
	c.Assert(err, check.IsNil)
	second := hex.EncodeToString(digest)
	c.Check(second, check.Not(check.Equals), first)

	c.Assert(os.Chmod(filepath.Join(root, "file"), 0o664), check.IsNil)

	digest, err = osutil.HashDirEntries(sha3.New384(), root)
	c.Assert(err, check.IsNil)
	third := hex.EncodeToString(digest)
	c.Check(third, check.Not(check.Equals), second)
}

func (s *hashSuite) TestHashDirEntriesHandlesLinks(c *check.C) {
	root := c.MkDir()

	c.Assert(os.WriteFile(filepath.Join(root, "file"), []byte("foo"), os.ModePerm), check.IsNil)

	digest, err := osutil.HashDirEntries(sha3.New384(), root)
	c.Assert(err, check.IsNil)
	fileDigest := hex.EncodeToString(digest)

	c.Assert(os.Remove(filepath.Join(root, "file")), check.IsNil)
	c.Assert(os.Symlink("foo", filepath.Join(root, "file")), check.IsNil)

	digest, err = osutil.HashDirEntries(sha3.New384(), root)
	c.Assert(err, check.IsNil)
	linkDigest := hex.EncodeToString(digest)
	c.Check(linkDigest, check.Not(check.Equals), fileDigest)
}

// Check that lengths of non-ASCII strings and directory file modes are
// calculated correctly.
func (s *hashSuite) TestHashDirEntriesMeasuresLengths(c *check.C) {
	root := c.MkDir()

	c.Assert(os.Mkdir(filepath.Join(root, "dir"), 0o755), check.IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "file"), []byte("üńîċōđę"), 0o644), check.IsNil)
	c.Assert(os.Symlink("ŨŃĪĈŐĐĘ", filepath.Join(root, "link↦")), check.IsNil)

	var metadata []byte

	hash := sha3.New384()
	_, _ = hash.Write([]byte("tree 0\x00"))
	// Mode length is only 5 bytes for directories.
	metadata = append(metadata, []byte("40755 dir\x00")...)
	metadata = hash.Sum(metadata)

	hash.Reset()
	// Blob length is 7 codepoints but 14 bytes.
	fmt.Fprintf(hash, "blob 14\x00%s", "üńîċōđę")
	metadata = append(metadata, []byte("100644 file\x00")...)
	metadata = hash.Sum(metadata)

	hash.Reset()
	// Blob length is 7 codepoints but 14 bytes.
	fmt.Fprintf(hash, "blob 14\x00%s", "ŨŃĪĈŐĐĘ")
	metadata = append(metadata, []byte("120000 link↦\x00")...)
	metadata = hash.Sum(metadata)

	hash.Reset()
	fmt.Fprintf(hash, "tree %d\x00", len(metadata))
	_, _ = hash.Write(metadata)
	expected := hex.EncodeToString(hash.Sum(nil))

	hash.Reset()
	digest, err := osutil.HashDirEntries(hash, root)
	c.Assert(err, check.IsNil)
	c.Check(hex.EncodeToString(digest), check.Equals, expected)
}
