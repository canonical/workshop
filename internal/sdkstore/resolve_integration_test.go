package sdkstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/sdkstore/transport"
	"github.com/canonical/workshop/internal/testutil"
)

type resolveIntegration struct{}

var _ = check.Suite(&resolveIntegration{})

func (f *resolveIntegration) TestResolveByName(c *check.C) {
	l, err := logger.New(os.Stderr, 0)
	c.Assert(err, check.IsNil)
	logger.SetLogger(l)

	req := transport.ResolveRequest{
		Packages: []transport.ResolvePackage{{
			InstanceKey: "random123",
			Namespace:   "sdk",
			Name:        "test-sdk-info-multi-base-1",
			Channel:     "latest/edge",
			Platform: transport.Platform{
				Name:         "ubuntu",
				Channel:      "22.04",
				Architecture: "riscv64",
			},
		}},
	}

	for range 1000 {
		client := NewClient(Config{})
		response, err := client.Resolve(context.Background(), req)
		c.Assert(err, check.IsNil)

		var expected any
		err = json.Unmarshal(testResolveNameResponse, &expected)
		c.Assert(err, check.IsNil)
		c.Check(response, testutil.JsonEquals, expected)
	}
}

func (f *resolveIntegration) TestResolveByID(c *check.C) {
	req := transport.ResolveRequest{
		Packages: []transport.ResolvePackage{{
			InstanceKey: "random456",
			Namespace:   "sdk",
			ID:          "x2hQwbuopxeELn5p1G9h3b9ZB3XM0JeG",
			Channel:     "latest/stable",
			Platform: transport.Platform{
				Name:         "ubuntu",
				Channel:      "20.04",
				Architecture: "amd64",
			},
		}},
	}

	client := NewClient(Config{})
	var response any
	err := client.resolve(context.Background(), &response, req)
	c.Assert(err, check.IsNil)

	var expected any
	err = json.Unmarshal(testResolveIDRaw, &expected)
	c.Assert(err, check.IsNil)
	c.Check(response, check.DeepEquals, expected)
}

func (f *resolveIntegration) TestResolveNotFound(c *check.C) {
	req := transport.ResolveRequest{
		Packages: []transport.ResolvePackage{{
			InstanceKey: "random789",
			Namespace:   "sdk",
			Name:        "not-found",
			Channel:     "latest/stable",
			Platform: transport.Platform{
				Name:         "ubuntu",
				Channel:      "24.04",
				Architecture: "s390x",
			},
		}},
	}

	client := NewClient(Config{})
	var response any
	err := client.resolve(context.Background(), &response, req)
	c.Assert(err, check.IsNil)

	var expected any
	err = json.Unmarshal(testResolveNotFoundRaw, &expected)
	c.Assert(err, check.IsNil)
	c.Check(response, check.DeepEquals, expected)
}

func (f *resolveIntegration) TestResolvePlatformWorkaround(c *check.C) {
	req := transport.ResolveRequest{
		Packages: []transport.ResolvePackage{{
			InstanceKey: "baseAndArch",
			Namespace:   "sdk",
			Name:        "go",
			Channel:     "1.25/stable",
			Platform: transport.Platform{
				Name:         "ubuntu",
				Channel:      "24.04",
				Architecture: "amd64",
			},
		}, {
			InstanceKey: "baseOnly",
			Namespace:   "sdk",
			Name:        "go",
			Channel:     "1.25/stable",
			Platform: transport.Platform{
				Name:         "ubuntu",
				Channel:      "24.04",
				Architecture: "all",
			},
		}, {
			InstanceKey: "archOnly",
			Namespace:   "sdk",
			Name:        "go",
			Channel:     "1.25/stable",
			Platform: transport.Platform{
				Name:         "all",
				Channel:      "all",
				Architecture: "amd64",
			},
		}, {
			InstanceKey: "neither",
			Namespace:   "sdk",
			Name:        "go",
			Channel:     "1.25/stable",
			Platform: transport.Platform{
				Name:         "all",
				Channel:      "all",
				Architecture: "all",
			},
		}},
	}

	client := NewClient(Config{})
	resp, err := client.Resolve(context.Background(), req)
	c.Assert(err, check.IsNil)

	var summary []string
	for _, pkg := range resp.PackageResults {
		var code string
		if pkg.Error != nil {
			code = string(pkg.Error.Code)
		}
		summary = append(summary, fmt.Sprintf("%s: %s%s", pkg.InstanceKey, pkg.Result.Channel.EffectiveChannel, code))
	}
	c.Check(summary, testutil.DeepUnsortedMatches, []string{
		"baseAndArch: revision-not-found",
		"baseOnly: revision-not-found",
		"archOnly: 1.25/stable",
		"neither: revision-not-found",
	})
}
