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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/lxd/shared/units"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/workshop"
)

var (
	ConnectSimpleStreams = lxd.ConnectSimpleStreams
	imageServer          = "simplestreams:https://cloud-images.ubuntu.com/releases"

	// However many backend instances are created, downloads are always a single
	// instance map with the LXD backend.
	imageLock        sync.Mutex
	currentDownloads = map[string]*downloadOp{}
)

func (b *Backend) GetBase(ctx context.Context, base string, confinement workshop.Confinement) (workshop.BaseImage, error) {
	source, err := baseImageSource(base)
	if err != nil {
		return workshop.BaseImage{}, err
	}

	imageServer, err := connectImageServer(*source)
	if err != nil {
		return workshop.BaseImage{}, err
	}
	defer imageServer.Disconnect()

	alias, _, err := imageServer.GetImageAliasType(source.ImageType, source.Alias)
	if err != nil {
		return workshop.BaseImage{}, fmt.Errorf("base %q not found: %w", base, err)
	}

	return workshop.BaseImage{Name: base, Confinement: confinement, Fingerprint: alias.Target}, nil
}

func (b *Backend) DownloadBase(ctx context.Context, image workshop.BaseImage, report *progress.Reporter) error {
	defer func() {
		if report != nil {
			imageLock.Lock()
			if op, exist := currentDownloads[image.Fingerprint]; exist {
				op.RemoveReporter(report.Name)
			}
			imageLock.Unlock()
		}
	}()

	imageLock.Lock()
	op, exist := currentDownloads[image.Fingerprint]
	if exist {
		if report != nil {
			op.AddReporter(report)
		}
		imageLock.Unlock()
		return waitDownloadOp(ctx, op)
	}

	op = newImageDownloadOp()
	if report != nil {
		op.AddReporter(report)
	}
	currentDownloads[image.Fingerprint] = op
	imageLock.Unlock()

	go b.tryDownloadBase(ctx, op, image)

	return waitDownloadOp(ctx, op)
}

func (b *Backend) tryDownloadBase(ctx context.Context, op *downloadOp, image workshop.BaseImage) {
	op.err = b.downloadBase(ctx, op, image)
	close(op.waitCh)

	imageLock.Lock()
	delete(currentDownloads, image.Fingerprint)
	imageLock.Unlock()
}

func (b *Backend) downloadBase(ctx context.Context, op *downloadOp, image workshop.BaseImage) error {
	source, err := baseImageSource(image.Name)
	if err != nil {
		return err
	}

	imageServer, err := connectImageServer(*source)
	if err != nil {
		return err
	}
	defer imageServer.Disconnect()

	// TODO: Remove this query once LXD starts reporting more detailed
	// progress information.
	imageInfo, _, err := imageServer.GetImage(image.Fingerprint)
	if err != nil {
		return fmt.Errorf("%q download failed: %w", image.Name, err)
	}

	req := api.ImagesPost{
		ImagePut: api.ImagePut{
			Properties: map[string]string{"workshop-base": image.Name},
		},
		Source: &api.ImagesPostSource{
			ImageSource: *source,
			Fingerprint: image.Fingerprint,
			Type:        api.SourceTypeImage,
		},
	}

	info, err := imageServer.GetConnectionInfo()
	if err != nil {
		return err
	}
	req.Source.Certificate = info.Certificate

	if !imageInfo.Public && source.Protocol != "simplestreams" {
		secret, err := imageServer.GetImageSecret(image.Fingerprint)
		if err != nil {
			return err
		}

		req.Source.Secret = secret
	}

	// LXD cannot cancel download operations
	child := context.WithoutCancel(ctx)

	conn, err := b.LxdClient(child)
	if err != nil {
		return err
	}
	defer conn.Disconnect()

	// Reset the connection's project to the default. Without this line,
	// the image is still copied to the default project, because our
	// projects specify features.images=false. The reason we reset the
	// project is to ensure that the downloaded image has cached=false.
	// This tells LXD not to prune the image when it expires. If the image
	// already exists with cached=true, then CopyImage only unsets it if
	// the connection uses the default project.
	// TODO: remove this when we remove features.images=false.
	conn = conn.UseProject("")

	copyop, err := conn.CreateImage(req, nil)
	if err != nil {
		return fmt.Errorf("%q download failed: %w", image.Name, err)
	}

	copyop.AddHandler(func(o api.Operation) {
		if o.Metadata == nil || imageInfo.Size <= 0 {
			return
		}
		if upd := handleImageUpdate(o.Metadata, imageInfo.Size); upd != nil {
			op.Update(*upd)
		}
	})

	if err := copyop.Wait(); err != nil {
		return fmt.Errorf("%q download failed: %w", image.Name, err)
	}

	return nil
}

func baseImageSource(base string) (*api.ImageSource, error) {
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '@' })
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid base %q (expected <NAME>@<VERSION>)", base)
	}
	if parts[0] != "ubuntu" {
		return nil, fmt.Errorf("base %q not supported", base)
	}

	protocol, url, found := strings.Cut(imageServer, ":")
	if !found {
		return nil, fmt.Errorf("invalid image server URL %q", imageServer)
	}

	// As of LXD 6.5, images are given 2 aliases, <NAME> and <NAME>/<ARCH>.
	// The more reliable one is <NAME>. The reason is that <ARCH> is taken
	// from api.Image.Properties["architecture"], which in turn comes
	// directly from the image server without any normalization (e.g. the
	// server could use amd64 or x86_64, we don't know). To choose which
	// image to abbreviate as <NAME>, LXD compares api.Image.Architecture
	// (which is normalized using LXD's osarch package) with `uname -m`.
	// There's a risk this doesn't work correctly for some 32-bit ARM
	// variants, but it should be OK for x86_64, aarch64 and riscv64.
	source := api.ImageSource{
		Alias:     parts[1],
		ImageType: string(api.InstanceTypeContainer),
		Protocol:  protocol,
		Server:    url,
	}
	return &source, nil
}

// For the test purposes only
func lxdConnectionArgs() (*lxd.ConnectionArgs, error) {
	args := &lxd.ConnectionArgs{}

	// Server certificate
	scrt := filepath.Join(dirs.WorkshopTlsDir, "server.crt")
	if osutil.FileExists(scrt) {
		content, err := os.ReadFile(scrt)
		if err != nil {
			return nil, err
		}

		args.TLSServerCert = string(content)
	}

	// Client certificate
	ccrt := filepath.Join(dirs.WorkshopTlsDir, "client.crt")
	if osutil.FileExists(ccrt) {
		content, err := os.ReadFile(ccrt)
		if err != nil {
			return nil, err
		}

		args.TLSClientCert = string(content)
	}

	// Client CA
	cca := filepath.Join(dirs.WorkshopTlsDir, "client.ca")
	if osutil.FileExists(cca) {
		content, err := os.ReadFile(cca)
		if err != nil {
			return nil, err
		}

		args.TLSCA = string(content)
	}

	// Client key
	ckey := filepath.Join(dirs.WorkshopTlsDir, "client.key")
	if osutil.FileExists(ckey) {
		content, err := os.ReadFile(ckey)
		if err != nil {
			return nil, err
		}

		pemKey, _ := pem.Decode(content)
		// Golang has deprecated all methods relating to PEM encryption due to a vulnerability.
		// However, the weakness does not make PEM unsafe for our purposes as it pertains to password protection on the
		// key file (client.key is only readable to the user in any case), so we'll ignore deprecation.
		if x509.IsEncryptedPEMBlock(pemKey) { //nolint:staticcheck
			return nil, fmt.Errorf("private key is password protected and no helper was configured")
		}

		args.TLSClientKey = string(content)
	}
	return args, nil
}

func connectImageServer(source api.ImageSource) (lxd.ImageServer, error) {
	switch source.Protocol {
	case "simplestreams":
		conn, err := ConnectSimpleStreams(source.Server, &lxd.ConnectionArgs{CachePath: dirs.BaseDownloads})
		if err != nil {
			return nil, fmt.Errorf("image server is not available: %w", err)
		}
		return conn, err
	case "lxd":
		args, err := lxdConnectionArgs()
		if err != nil {
			return nil, err
		}
		conn, err := lxd.ConnectPublicLXD(source.Server, args)
		if err != nil {
			return nil, fmt.Errorf("image server is not available: %w", err)
		}
		return conn, err
	default:
		return nil, fmt.Errorf("unknown image server URL prefix (supported: simplestreams, lxd)")
	}
}

func waitDownloadOp(ctx context.Context, op *downloadOp) error {
	select {
	case <-ctx.Done():
		// Do not try to cancel the target op here as LXD is unable to cancel
		// image download properly. Instead, we'll wait for it to finish if
		// the task will be restarted.
		return ctx.Err()
	case <-op.waitCh:
		return op.err
	}
}

var (
	// rootfs: 95% (37.46MB/s)
	imgDownloadSS = regexp.MustCompile(`^(?:rootfs(?: delta)?: )?(?P<done>[0-9]+)% (?P<speed>\([\w/\.]+\))$`)
	// 225.19MB (37.46MB/s)
	imgDownloadLXD = regexp.MustCompile(`^(?P<done>[0-9\.]+)(?P<mult>\w+) (?P<speed>\([\w/\.]+\))$`)
)

// handleLaunchUpdate parses a LXD create instance operation metadata and
// reports the operation's progress if available. The LXD metadata is
// inconsistent between operations that handleLaunchUpdate accommodates by
// looking for specific progress labels. NOTE: There is no guarantee that the
// LXD's progress reporting format won't change; this meta data parser is valid
// for LXD 6.5.
func handleImageUpdate(opmeta map[string]any, imsize int64) *downloadUpdate {
	upd, ok := opmeta["download_progress"].(string)
	if !ok {
		return nil
	}

	// check if the response metadata comes from a simplestream protocol
	if data := imgDownloadSS.FindStringSubmatch(upd); len(data) == 3 {
		done, err := strconv.ParseInt(data[1], 10, 0)
		if err != nil {
			// just in case, but this is ensured by the regex
			return nil
		}
		// now, "done" is the percentage value. The state.Task progress
		// reporting expects to have bytes all the way up to the client
		// to calculate the download speed. Thus, convert percentages to
		// bytes.
		donebytes := imsize * done / 100
		return &downloadUpdate{Label: "download", Done: donebytes, Total: imsize}
	}

	// check if the response metadata comes from a lxd protocol
	if data := imgDownloadLXD.FindStringSubmatch(upd); len(data) == 4 {
		done, err := strconv.ParseFloat(data[1], 32)
		if err != nil {
			return nil
		}
		// ParseByteSizeString understands only int, so we use it to get
		// a multiplier for "done".
		multiplier, err := units.ParseByteSizeString("1" + data[2])
		if err != nil {
			return nil
		}
		donebytes := int64(done) * multiplier
		return &downloadUpdate{Label: "download", Done: donebytes, Total: imsize}
	}

	return nil
}

type downloadUpdate struct {
	Label string
	Done  int64
	Total int64
}

type downloadOp struct {
	waitCh chan struct{}
	err    error

	reportersLock sync.Mutex
	reporters     map[string]*progress.Reporter
}

func newImageDownloadOp() *downloadOp {
	return &downloadOp{waitCh: make(chan struct{}), reporters: make(map[string]*progress.Reporter, 0)}
}

func (r *downloadOp) AddReporter(rep *progress.Reporter) {
	r.reportersLock.Lock()
	defer r.reportersLock.Unlock()

	r.reporters[rep.Name] = rep
}

func (r *downloadOp) RemoveReporter(name string) {
	r.reportersLock.Lock()
	defer r.reportersLock.Unlock()
	delete(r.reporters, name)
}

func (r *downloadOp) Update(upd downloadUpdate) {
	r.reportersLock.Lock()
	defer r.reportersLock.Unlock()

	for _, rep := range r.reporters {
		rep.Report(upd.Label, upd.Done, upd.Total)
	}
}

func FakeImageServer(server string) func() {
	oldImageServer := imageServer
	imageServer = server
	return func() { imageServer = oldImageServer }
}
