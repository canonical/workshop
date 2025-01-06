package store

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"

	"github.com/canonical/workshop/internal/logger"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/progress"
	"github.com/canonical/workshop/internal/sdk"
)

var (
	ErrNoRefreshAvailable = errors.New("SDK has no update available")
)

var (
	SDK_STORE_BUCKET_NAME = "sdk-store"

	storeSdkInfo   = storeSdkInfoImpl
	storeConnect   = storeConnectImpl
	storeSdkReader = storeSdkReaderImpl
)

func New() *GcsStore {
	return &GcsStore{}
}

type GcsStore struct {
}

type storeSdk struct {
	Name     string       `json:"name"`
	Channel  string       `json:"channel"`
	Revision sdk.Revision `json:"revision"`
	SdkYAML  string       `json:"sdk-yaml"`
	Size     int64        `json:"size"`
}

type SdkActionError struct {
	// maps an SDK name to an error
	errors map[string]error
}

func (e SdkActionError) Error() string {
	errorMsg := []string{"SDK store action failed:"}

	for name, e := range e.errors {
		errorMsg = append(errorMsg, "- "+name+": "+e.Error())
	}
	return strings.Join(errorMsg, "\n")
}

type ClientWrapper struct {
	*storage.Client
	isTesting bool
}

func (c *GcsStore) SdkAction(ctx context.Context, actions []sdk.SdkAction) ([]sdk.SdkResult, error) {
	results := []sdk.SdkResult{}
	actError := &SdkActionError{
		errors: make(map[string]error),
	}
	for _, act := range actions {
		switch act.Action {
		case sdk.Install:
			if act.Channel == "" {
				continue
			}
			s, err := storeSdkInfo(ctx, act.Name, act.Channel)
			if err != nil {
				actError.errors[act.Name] = err
				continue
			}

			info, err := sdk.ReadSdkInfo([]byte(s.SdkYAML), act.ProjectId, act.Workshop)
			if err != nil {
				actError.errors[act.Name] = err
				continue
			}
			info.Revision = s.Revision
			info.Channel = s.Channel

			results = append(results, sdk.SdkResult{Info: info})
		default:
			return nil, fmt.Errorf("unknown SDK store action")
		}
	}

	if len(actError.errors) > 0 {
		return results, actError
	}

	return results, nil
}

type reporterWriter struct {
	r     *progress.Reporter
	done  int
	total int
}

func (r *reporterWriter) Write(p []byte) (n int, err error) {
	plen := len(p)
	r.done += plen
	r.r.Report("download", r.done, r.total)
	return plen, nil
}

func (c *GcsStore) DownloadSdk(ctx context.Context, setup sdk.Setup, report *progress.Reporter) error {
	fl, err := sdk.OpenLock(setup.Name)
	if err != nil {
		return err
	}
	if err = fl.Lock(); err != nil {
		return err
	}
	defer fl.Close()

	info, err := storeSdkInfo(ctx, setup.Name, setup.Channel)
	if err != nil {
		return err
	}

	r, err := storeSdkReader(ctx, setup)
	if err != nil {
		return err
	}
	defer r.Close()

	target := setup.Filepath()
	if !osutil.FileExists(target) {
		file, err := os.Create(target)
		if err != nil {
			return err
		}
		defer func() {
			// Remove the target as due to the error it may be corrupted.
			if err != nil {
				if err1 := os.Remove(target); err1 != nil {
					logger.Noticef("SDK Store on Download: Cannot remove %q on a failed download: %v", target, err1)
				}
				return
			}
			// If the SDK was downloaded successfully, remove its previous rev if any.
			matches, err1 := filepath.Glob(filepath.Join(filepath.Dir(target), setup.Name+"_*.sdk"))
			if err1 != nil {
				logger.Noticef("SDK Store on Download: Cannot cleanup previous downloads for %q: %v", setup.Name, err1)
			}
			for _, m := range matches {
				if m != target {
					if err1 = os.Remove(m); err1 != nil {
						logger.Noticef("SDK Store on Download: Cannot cleanup previous download (%s): %v", m, err1)
					}
				}
			}
		}()
		defer file.Close()

		var writer io.Writer
		if report != nil {
			writer = io.MultiWriter(file, &reporterWriter{r: report, total: int(info.Size)})
		} else {
			writer = file
		}

		if _, err = io.Copy(writer, r); err != nil {
			return err
		}
	} else {
		logger.Debugf("SDK Store on Download: SDK %q found locally: %s", setup.Name, target)
	}

	return nil
}

func storeConnectImpl(ctx context.Context) (*ClientWrapper, error) {
	opt := option.WithoutAuthentication()
	testing := false
	if url := os.Getenv("SDK_STORE_URL"); url != "" { // Set STORAGE_EMULATOR_HOST environment variable for GSC.
		err := os.Setenv("STORAGE_EMULATOR_HOST", "localhost:8080")
		if err != nil {
			return nil, err
		}
		opt = option.WithEndpoint(url)
		testing = true
	}
	client, err := storage.NewClient(ctx, opt)
	if err != nil {
		return nil, err
	}
	return &ClientWrapper{client, testing}, nil
}

func storeSdkInfoImpl(ctx context.Context, name, channel string) (storeSdk, error) {
	var sSdk storeSdk
	client, err := storeConnect(ctx)
	if err != nil {
		return sSdk, err
	}
	defer client.Close()
	bkt := client.Bucket(SDK_STORE_BUCKET_NAME)

	var sa = strings.Split(channel, "/")
	if len(sa) != 2 {
		return sSdk, fmt.Errorf("%q has an invalid channel %q, must take the form <track>/<risk>", name, channel)
	}
	track, risk := sa[0], sa[1]
	obj := bkt.Object(fmt.Sprintf("%s/%s/%s/%s.sdk", name, track, risk, name))
	// Max attempts prevents workshop from trying to dial the store indefinitely.
	// Behavior is only modified for this specific Object Handle, it is not
	// passed on to SDK download behavior
	obj = obj.Retryer(
		storage.WithMaxAttempts(3),
	)
	atr, err := obj.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return sSdk, fmt.Errorf("SDK not found in %q", channel)
		}
		if urlErr, ok := err.(*url.Error); ok {
			opErr, ok := urlErr.Err.(*net.OpError)
			if ok {
				return sSdk, fmt.Errorf("cannot connect to store: %q", opErr)
			}
		}
		return sSdk, err
	}
	sSdk.Name = name
	sSdk.Channel = channel
	// A simple modulo to keep revision numbers in a readable form for testing
	sSdk.Revision = sdk.Revision{N: int(atr.Generation % 1000)}
	sSdk.Size = atr.Size
	// The test server for the SDK store cannot store metadata.
	if !client.isTesting {
		if _, ok := atr.Metadata["sdk-yaml"]; !ok {
			return sSdk, fmt.Errorf("SDK %q does not have metadata", name)
		}
		sSdk.SdkYAML = atr.Metadata["sdk-yaml"]
	} else {
		// Emulate meta data for the test SDKs. We need the name/base pair for
		// the e2e scenarios to work.
		sSdk.SdkYAML, err = readTestMetadata(ctx, client, name, track, risk)
		if err != nil {
			return sSdk, err
		}
	}
	return sSdk, nil
}

// Only relevant for the end to end tests
func readTestMetadata(ctx context.Context, client *ClientWrapper, name, track, risk string) (string, error) {
	var b bytes.Buffer
	meta := bufio.NewWriter(&b)

	bkt := client.Bucket(SDK_STORE_BUCKET_NAME)
	obj := bkt.Object(fmt.Sprintf("%s/%s/%s/sdk.yaml", name, track, risk))
	r, err := obj.NewReader(ctx)
	if err != nil {
		return "", err
	}
	defer r.Close()

	if _, err = io.Copy(meta, r); err != nil {
		return "", err
	}
	return b.String(), nil
}

func storeSdkReaderImpl(ctx context.Context, setup sdk.Setup) (io.ReadCloser, error) {
	client, err := storeConnect(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	var sa = strings.Split(setup.Channel, "/")
	if len(sa) != 2 {
		return nil, fmt.Errorf("%s has an invalid channel %s, must take the form <track>/<risk>", setup.Name, setup.Channel)
	}
	track, risk := sa[0], sa[1]
	bkt := client.Bucket(SDK_STORE_BUCKET_NAME)

	obj := bkt.Object(fmt.Sprintf("%s/%s/%s/%s.sdk", setup.Name, track, risk, setup.Name)).Retryer(
		storage.WithBackoff(gax.Backoff{Initial: 2 * time.Second}),
		storage.WithPolicy(storage.RetryIdempotent),
	)
	r, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, errors.New("SDK not found")
		}
		return nil, err
	}
	return r, nil
}
