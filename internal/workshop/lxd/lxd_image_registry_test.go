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

package lxdbackend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/testutil"
	"github.com/canonical/workshop/internal/workshop"
)

const (
	testRegistry          = "pkgproxy-ubuntu"
	testNobleContainerFP  = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	testNobleOlderFP      = "0000c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	testNobleVMFP         = "b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90a1"
	testNobleOtherArchFP  = "c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2"
	testJammyContainerFP  = "d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3"
	testRegistryImagesURL = "/1.0/image-registries/pkgproxy-ubuntu/images"
)

// testRegistryCatalog returns the images an image registry mirroring
// cloud-images.ubuntu.com/releases lists to an arm64 LXD server.
func testRegistryCatalog() []api.Image {
	day := func(d int) time.Time { return time.Date(2026, time.November, d, 0, 0, 0, 0, time.UTC) }
	return []api.Image{{
		Fingerprint:  testNobleOlderFP,
		Type:         "container",
		Architecture: "aarch64",
		Size:         100,
		CreatedAt:    day(1),
		Aliases:      []api.ImageAlias{{Name: "24.04"}, {Name: "24.04/arm64"}, {Name: "noble"}},
	}, {
		Fingerprint:  testNobleContainerFP,
		Type:         "container",
		Architecture: "aarch64",
		Size:         463470592,
		CreatedAt:    day(5),
		Aliases:      []api.ImageAlias{{Name: "24.04"}, {Name: "24.04/arm64"}, {Name: "noble"}},
	}, {
		Fingerprint:  testNobleVMFP,
		Type:         "virtual-machine",
		Architecture: "aarch64",
		Size:         612368384,
		CreatedAt:    day(5),
		Aliases:      []api.ImageAlias{{Name: "24.04"}, {Name: "24.04/arm64"}, {Name: "noble"}},
	}, {
		Fingerprint:  testNobleOtherArchFP,
		Type:         "container",
		Architecture: "x86_64",
		Size:         463470592,
		CreatedAt:    day(6),
		Aliases:      []api.ImageAlias{{Name: "24.04/amd64"}, {Name: "noble/amd64"}},
	}, {
		// LXD treats an image without a type as a container image.
		Fingerprint:  testJammyContainerFP,
		Architecture: "aarch64",
		Size:         423624704,
		CreatedAt:    day(5),
		Aliases:      []api.ImageAlias{{Name: "22.04"}, {Name: "jammy"}},
	}}
}

// fakeRegistryServer is the part of an LXD server that the image registry
// code uses. Calling any other method panics.
type fakeRegistryServer struct {
	lxd.InstanceServer

	extensions []string
	images     []api.Image
	queryErr   error

	queries      []string
	posts        []string
	postData     []any
	disconnected int
}

func (f *fakeRegistryServer) HasExtension(name string) bool {
	return slices.Contains(f.extensions, name)
}

func (f *fakeRegistryServer) RawQuery(method, path string, data any, etag string) (*api.Response, string, error) {
	f.queries = append(f.queries, method+" "+path)
	if f.queryErr != nil {
		return nil, "", f.queryErr
	}
	metadata, err := json.Marshal(f.images)
	if err != nil {
		return nil, "", err
	}
	return &api.Response{Type: api.SyncResponse, Metadata: metadata}, "", nil
}

func (f *fakeRegistryServer) RawOperation(method, path string, data any, etag string) (lxd.Operation, string, error) {
	f.posts = append(f.posts, method+" "+path)
	f.postData = append(f.postData, data)
	return &fakeOperation{}, "", nil
}

func (f *fakeRegistryServer) Disconnect() {
	f.disconnected++
}

// fakeOperation is an LXD operation that has already succeeded.
type fakeOperation struct {
	lxd.Operation
}

func (*fakeOperation) AddHandler(func(api.Operation)) (*lxd.EventTarget, error) {
	return nil, nil
}

func (*fakeOperation) Wait() error {
	return nil
}

// fakeSimpleStreams is a simplestreams server that knows one alias.
type fakeSimpleStreams struct {
	lxd.ImageServer
}

func (*fakeSimpleStreams) GetImageAliasType(imageType, name string) (*api.ImageAliasesEntry, string, error) {
	if imageType == "container" && name == "24.04" {
		return &api.ImageAliasesEntry{Name: name, Type: imageType, Target: testNobleContainerFP}, "", nil
	}
	return nil, "", errors.New("not found")
}

func (*fakeSimpleStreams) Disconnect() {}

type imageRegistrySuite struct {
	testutil.BaseTest

	server   *fakeRegistryServer
	connects int
}

var _ = check.Suite(&imageRegistrySuite{})

func (s *imageRegistrySuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)

	s.server = &fakeRegistryServer{
		extensions: []string{imageRegistriesExtension},
		images:     testRegistryCatalog(),
	}
	s.connects = 0

	oldConnect := connectLXDUnix
	connectLXDUnix = func(ctx context.Context) (lxd.InstanceServer, error) {
		s.connects++
		return s.server, nil
	}
	oldConnectSimpleStreams := ConnectSimpleStreams
	ConnectSimpleStreams = func(url string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Check(url, check.Equals, "https://cloud-images.ubuntu.com/releases")
		return &fakeSimpleStreams{}, nil
	}
	restoreServer := FakeImageServer("simplestreams:https://cloud-images.ubuntu.com/releases")
	restoreRegistry := FakeImageRegistry("")

	s.AddCleanup(func() {
		connectLXDUnix = oldConnect
		ConnectSimpleStreams = oldConnectSimpleStreams
		restoreServer()
		restoreRegistry()
	})
}

func (s *imageRegistrySuite) TestFindRegistryImage(c *check.C) {
	images := testRegistryCatalog()

	// The newest container image the alias names wins.
	image, err := findRegistryImage(images, "24.04", "container")
	c.Assert(err, check.IsNil)
	c.Check(image.Fingerprint, check.Equals, testNobleContainerFP)

	image, err = findRegistryImage(images, "24.04", "virtual-machine")
	c.Assert(err, check.IsNil)
	c.Check(image.Fingerprint, check.Equals, testNobleVMFP)

	// An image without a type is a container image.
	image, err = findRegistryImage(images, "22.04", "container")
	c.Assert(err, check.IsNil)
	c.Check(image.Fingerprint, check.Equals, testJammyContainerFP)

	_, err = findRegistryImage(images, "22.04", "virtual-machine")
	c.Check(err, check.ErrorMatches, `no virtual-machine image with alias "22.04"`)

	_, err = findRegistryImage(images, "26.04", "container")
	c.Check(err, check.ErrorMatches, `no container image with alias "26.04"`)

	_, err = findRegistryImage(nil, "24.04", "container")
	c.Check(err, check.ErrorMatches, `no container image with alias "24.04"`)
}

func (s *imageRegistrySuite) TestValidateImageRegistryName(c *check.C) {
	c.Check(validateImageRegistryName("pkgproxy-ubuntu"), check.IsNil)
	c.Check(validateImageRegistryName("ubuntu"), check.IsNil)

	for _, name := range []string{"", "a/b", "a?b", "a#b", "a b", `a\b`} {
		c.Check(validateImageRegistryName(name), check.ErrorMatches, `invalid image registry name ".*"`, check.Commentf("%q", name))
	}
}

func (s *imageRegistrySuite) TestRegistryImageRequest(c *check.C) {
	image := workshop.BaseImage{Name: "ubuntu@24.04", Runtime: workshop.RuntimeLXDVM, Fingerprint: testNobleVMFP}

	data, err := json.Marshal(registryImageRequest(testRegistry, image))
	c.Assert(err, check.IsNil)

	var req map[string]any
	c.Assert(json.Unmarshal(data, &req), check.IsNil)

	c.Check(req["properties"], check.DeepEquals, map[string]any{"workshop-base": "ubuntu@24.04"})
	source, ok := req["source"].(map[string]any)
	c.Assert(ok, check.Equals, true)
	c.Check(source["image_registry"], check.Equals, testRegistry)
	c.Check(source["type"], check.Equals, "image")
	c.Check(source["mode"], check.Equals, "pull")
	c.Check(source["fingerprint"], check.Equals, testNobleVMFP)
	c.Check(source["image_type"], check.Equals, "virtual-machine")
	// LXD resolves the server through the registry, not a URL.
	c.Check(source["server"], check.Equals, "")
	c.Check(source["protocol"], check.Equals, "")
}

func (s *imageRegistrySuite) TestGetBaseWithoutRegistry(c *check.C) {
	b := &Backend{}

	image, err := b.GetBase(context.Background(), "ubuntu@24.04", workshop.RuntimeLXDContainer)
	c.Assert(err, check.IsNil)
	c.Check(image, check.Equals, workshop.BaseImage{
		Name:        "ubuntu@24.04",
		Runtime:     workshop.RuntimeLXDContainer,
		Fingerprint: testNobleContainerFP,
	})

	// Workshop asks the image server itself, not LXD.
	c.Check(s.connects, check.Equals, 0)
}

func (s *imageRegistrySuite) TestGetBaseFromRegistry(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	b := &Backend{}

	image, err := b.GetBase(context.Background(), "ubuntu@24.04", workshop.RuntimeLXDContainer)
	c.Assert(err, check.IsNil)
	c.Check(image, check.Equals, workshop.BaseImage{
		Name:        "ubuntu@24.04",
		Runtime:     workshop.RuntimeLXDContainer,
		Fingerprint: testNobleContainerFP,
	})

	image, err = b.GetBase(context.Background(), "ubuntu@24.04", workshop.RuntimeLXDVM)
	c.Assert(err, check.IsNil)
	c.Check(image.Fingerprint, check.Equals, testNobleVMFP)

	c.Check(s.server.queries, check.DeepEquals, []string{
		"GET " + testRegistryImagesURL,
		"GET " + testRegistryImagesURL,
	})
	c.Check(s.server.disconnected, check.Equals, 2)
}

func (s *imageRegistrySuite) TestGetBaseFromRegistryNotFound(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	b := &Backend{}

	_, err := b.GetBase(context.Background(), "ubuntu@26.04", workshop.RuntimeLXDContainer)
	c.Check(err, check.ErrorMatches, `base "ubuntu@26.04" not found in image registry "pkgproxy-ubuntu": no container image with alias "26.04"`)
}

func (s *imageRegistrySuite) TestGetBaseFromRegistryInvalidBase(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	b := &Backend{}

	_, err := b.GetBase(context.Background(), "debian@13", workshop.RuntimeLXDContainer)
	c.Check(err, check.ErrorMatches, `base "debian@13" not supported`)
	c.Check(s.connects, check.Equals, 0)
}

func (s *imageRegistrySuite) TestGetBaseFromRegistryQueryError(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	s.server.queryErr = errors.New(`Image registry "pkgproxy-ubuntu" not found`)
	b := &Backend{}

	_, err := b.GetBase(context.Background(), "ubuntu@24.04", workshop.RuntimeLXDContainer)
	c.Check(err, check.ErrorMatches, `cannot list images of image registry "pkgproxy-ubuntu": Image registry "pkgproxy-ubuntu" not found`)
}

func (s *imageRegistrySuite) TestGetBaseFromRegistryWithoutExtension(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	s.server.extensions = nil
	b := &Backend{}

	_, err := b.GetBase(context.Background(), "ubuntu@24.04", workshop.RuntimeLXDContainer)
	c.Check(err, check.ErrorMatches, `cannot use image registry "pkgproxy-ubuntu": LXD server lacks the "image_registries" API extension \(LXD 6.10 or later\)`)
	c.Check(s.server.queries, check.HasLen, 0)
}

func (s *imageRegistrySuite) TestGetBaseFromRegistryInvalidName(c *check.C) {
	defer FakeImageRegistry("../images")()
	b := &Backend{}

	_, err := b.GetBase(context.Background(), "ubuntu@24.04", workshop.RuntimeLXDContainer)
	c.Check(err, check.ErrorMatches, `invalid image registry name "../images"`)
	c.Check(s.server.queries, check.HasLen, 0)
}

func (s *imageRegistrySuite) TestDownloadBaseFromRegistry(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	b := &Backend{}
	image := workshop.BaseImage{Name: "ubuntu@24.04", Runtime: workshop.RuntimeLXDContainer, Fingerprint: testNobleContainerFP}

	err := b.DownloadBase(context.Background(), image, nil)
	c.Assert(err, check.IsNil)

	c.Check(s.server.posts, check.DeepEquals, []string{"POST /images"})
	c.Assert(s.server.postData, check.HasLen, 1)
	c.Check(s.server.postData[0], check.DeepEquals, registryImageRequest(testRegistry, image))
	c.Check(s.server.disconnected, check.Equals, 1)
}

func (s *imageRegistrySuite) TestDownloadBaseFromRegistryWithoutExtension(c *check.C) {
	defer FakeImageRegistry(testRegistry)()
	s.server.extensions = []string{"image_types"}
	b := &Backend{}
	image := workshop.BaseImage{Name: "ubuntu@24.04", Runtime: workshop.RuntimeLXDContainer, Fingerprint: testNobleContainerFP}

	err := b.DownloadBase(context.Background(), image, nil)
	c.Check(err, check.ErrorMatches, `cannot use image registry "pkgproxy-ubuntu": LXD server lacks the "image_registries" API extension \(LXD 6.10 or later\)`)
	c.Check(s.server.posts, check.HasLen, 0)
}

func (s *imageRegistrySuite) TestLoadEnvConfigImageRegistry(c *check.C) {
	setenv(&s.BaseTest, "WORKSHOP_IMAGE_SERVER", "")
	setenv(&s.BaseTest, "WORKSHOP_IMAGE_REGISTRY", "")
	loadEnvConfig()
	c.Check(imageRegistry, check.Equals, "")
	c.Check(imageServer, check.Equals, "simplestreams:https://cloud-images.ubuntu.com/releases")

	setenv(&s.BaseTest, "WORKSHOP_IMAGE_REGISTRY", testRegistry)
	loadEnvConfig()
	c.Check(imageRegistry, check.Equals, testRegistry)
	c.Check(imageServer, check.Equals, "simplestreams:https://cloud-images.ubuntu.com/releases")
}

// setenv sets the environment variable key to value until the test
// finishes.
func setenv(s *testutil.BaseTest, key, value string) {
	old, ok := os.LookupEnv(key)
	s.AddCleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
	_ = os.Setenv(key, value)
}
