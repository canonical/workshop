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
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"

	"github.com/canonical/workshop/internal/workshop"
)

// imageRegistriesExtension is the LXD API extension, added in LXD 6.10,
// that makes LXD download images through image registries.
const imageRegistriesExtension = "image_registries"

// connectLXDUnix connects to the local LXD server, in its default project.
var connectLXDUnix = func(ctx context.Context) (lxd.InstanceServer, error) {
	conn, err := lxd.ConnectLXDUnixWithContext(ctx, "", nil)
	if err != nil {
		return nil, ErrorLxdBackend(err)
	}
	return conn, nil
}

// registryImagesPostSource is api.ImagesPostSource with the image registry
// field of the "image_registries" API extension, which the LXD client
// Workshop builds with predates.
type registryImagesPostSource struct {
	api.ImagesPostSource

	// ImageRegistry names the image registry LXD resolves and downloads
	// the image through.
	ImageRegistry string `json:"image_registry"`
}

// registryImagesPost is api.ImagesPost with a registryImagesPostSource
// source. Its Source field shadows the one of api.ImagesPost.
type registryImagesPost struct {
	api.ImagesPost

	Source *registryImagesPostSource `json:"source"`
}

// validateImageRegistryName checks that name can be used in the path of an
// LXD API request.
func validateImageRegistryName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\?# \t\n") {
		return fmt.Errorf("invalid image registry name %q", name)
	}
	return nil
}

// checkImageRegistries checks that the LXD server behind conn can resolve
// and download images through the image registry named registry.
func checkImageRegistries(conn lxd.InstanceServer, registry string) error {
	if err := validateImageRegistryName(registry); err != nil {
		return err
	}
	if !conn.HasExtension(imageRegistriesExtension) {
		return fmt.Errorf("cannot use image registry %q: LXD server lacks the %q API extension (LXD 6.10 or later)", registry, imageRegistriesExtension)
	}
	return nil
}

// registryImages returns the images LXD finds through the image registry
// named registry, as "lxc image list --registry" lists them.
func registryImages(conn lxd.InstanceServer, registry string) ([]api.Image, error) {
	path := "/1.0/image-registries/" + url.PathEscape(registry) + "/images"
	resp, _, err := conn.RawQuery(http.MethodGet, path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("cannot list images of image registry %q: %w", registry, err)
	}

	var images []api.Image
	if err := resp.MetadataAsStruct(&images); err != nil {
		return nil, fmt.Errorf("cannot list images of image registry %q: %w", registry, err)
	}
	return images, nil
}

// findRegistryImage returns the image of type imageType that alias names
// among images. LXD gives the bare alias (as opposed to <ALIAS>/<ARCH>)
// only to images of the LXD server's own architecture. Should several
// images match, the newest one is returned.
func findRegistryImage(images []api.Image, alias, imageType string) (*api.Image, error) {
	var found *api.Image
	for i := range images {
		image := &images[i]

		// LXD treats an image without a type as a container image.
		typ := image.Type
		if typ == "" {
			typ = string(api.InstanceTypeContainer)
		}
		if typ != imageType {
			continue
		}

		hasAlias := slices.ContainsFunc(image.Aliases, func(a api.ImageAlias) bool {
			return a.Name == alias
		})
		if !hasAlias {
			continue
		}

		if found == nil || image.CreatedAt.After(found.CreatedAt) {
			found = image
		}
	}

	if found == nil {
		return nil, fmt.Errorf("no %s image with alias %q", imageType, alias)
	}
	return found, nil
}

// getRegistryBase resolves base through imageRegistry.
func (b *Backend) getRegistryBase(ctx context.Context, base string, runtime workshop.Runtime) (workshop.BaseImage, error) {
	alias, err := baseImageAlias(base)
	if err != nil {
		return workshop.BaseImage{}, err
	}

	conn, err := connectLXDUnix(ctx)
	if err != nil {
		return workshop.BaseImage{}, err
	}
	defer conn.Disconnect()

	if err := checkImageRegistries(conn, imageRegistry); err != nil {
		return workshop.BaseImage{}, err
	}

	images, err := registryImages(conn, imageRegistry)
	if err != nil {
		return workshop.BaseImage{}, err
	}

	image, err := findRegistryImage(images, alias, string(instanceType(runtime)))
	if err != nil {
		return workshop.BaseImage{}, fmt.Errorf("base %q not found in image registry %q: %w", base, imageRegistry, err)
	}

	return workshop.BaseImage{Name: base, Runtime: runtime, Fingerprint: image.Fingerprint}, nil
}

// registryImageRequest returns the request that makes LXD download the
// image through registry, as "lxc image copy <REGISTRY>:<IMAGE> local:"
// does on LXD 6.10.
func registryImageRequest(registry string, image workshop.BaseImage) registryImagesPost {
	return registryImagesPost{
		ImagesPost: api.ImagesPost{
			ImagePut: api.ImagePut{
				Properties: map[string]string{"workshop-base": image.Name},
			},
		},
		Source: &registryImagesPostSource{
			ImagesPostSource: api.ImagesPostSource{
				ImageSource: api.ImageSource{
					ImageType: string(instanceType(image.Runtime)),
				},
				Mode:        "pull",
				Type:        api.SourceTypeImage,
				Fingerprint: image.Fingerprint,
			},
			ImageRegistry: registry,
		},
	}
}

// downloadRegistryBase makes LXD download image through imageRegistry.
func (b *Backend) downloadRegistryBase(ctx context.Context, op *downloadOp, image workshop.BaseImage) error {
	// LXD cannot cancel download operations
	child := context.WithoutCancel(ctx)

	// The connection uses the default project, so that the downloaded
	// image has cached=false and LXD does not prune it when it expires
	// (see downloadBase).
	conn, err := connectLXDUnix(child)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	if err := checkImageRegistries(conn, imageRegistry); err != nil {
		return err
	}

	// TODO: Remove this query once LXD starts reporting more detailed
	// progress information.
	images, err := registryImages(conn, imageRegistry)
	if err != nil {
		return fmt.Errorf("%q download failed: %w", image.Name, err)
	}
	var size int64
	if idx := slices.IndexFunc(images, func(i api.Image) bool { return i.Fingerprint == image.Fingerprint }); idx >= 0 {
		size = images[idx].Size
	}

	copyop, _, err := conn.RawOperation(http.MethodPost, "/images", registryImageRequest(imageRegistry, image), "")
	if err != nil {
		return fmt.Errorf("%q download failed: %w", image.Name, err)
	}
	reportImageDownload(copyop, op, size)

	if err := copyop.Wait(); err != nil {
		return fmt.Errorf("%q download failed: %w", image.Name, err)
	}

	return nil
}
