// Package gcs implements Storage based on Google Cloud Storage bucket.
package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	gcsclient "cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/kopia/kopia/internal/clock"
	"github.com/kopia/kopia/internal/iocopy"
	"github.com/kopia/kopia/internal/timestampmeta"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/retrying"
)

const (
	gcsStorageType  = "gcs"
	writerChunkSize = 1 << 20
	latestVersionID = ""

	timeMapKey = "Kopia-Mtime" // case is important, first letter must be capitalized.
)

type gcsStorage struct {
	Options
	blob.DefaultProviderImplementation

	storageClient *gcsclient.Client
	bucket        *gcsclient.BucketHandle
}

func (gcs *gcsStorage) GetBlob(ctx context.Context, b blob.ID, offset, length int64, output blob.OutputBuffer) error {
	return gcs.getBlobWithVersion(ctx, b, latestVersionID, offset, length, output)
}

// getBlobWithVersion returns full or partial contents of a blob with given ID and version.
func (gcs *gcsStorage) getBlobWithVersion(ctx context.Context, b blob.ID, version string, offset, length int64, output blob.OutputBuffer) error {
	if offset < 0 {
		return blob.ErrInvalidRange
	}

	obj := gcs.bucket.Object(gcs.getObjectNameString(b))

	if version != "" {
		gen, err := strconv.ParseInt(version, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse blob version")
		}

		obj = obj.Generation(gen)
	}

	attempt := func() error {
		reader, err := obj.NewRangeReader(ctx, offset, length)
		if err != nil {
			return errors.Wrap(err, "NewRangeReader")
		}
		defer reader.Close() //nolint:errcheck

		return iocopy.JustCopy(output, reader)
	}

	if err := attempt(); err != nil {
		return translateError(err)
	}

	//nolint:wrapcheck
	return blob.EnsureLengthExactly(output.Length(), length)
}

func (gcs *gcsStorage) GetMetadata(ctx context.Context, b blob.ID) (blob.Metadata, error) {
	objName := gcs.getObjectNameString(b)
	obj := gcs.bucket.Object(objName)

	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return blob.Metadata{}, errors.Wrap(translateError(err), "Attrs")
	}

	return gcs.getBlobMeta(attrs), nil
}

func (gcs *gcsStorage) getBlobMeta(attrs *gcsclient.ObjectAttrs) blob.Metadata {
	bm := blob.Metadata{
		BlobID:    gcs.toBlobID(attrs.Name),
		Length:    attrs.Size,
		Timestamp: attrs.Created,
	}

	if t, ok := timestampmeta.FromValue(attrs.Metadata[timeMapKey]); ok {
		bm.Timestamp = t
	}

	return bm
}

func translateError(err error) error {
	var ae *googleapi.Error

	if errors.As(err, &ae) {
		switch ae.Code {
		case http.StatusRequestedRangeNotSatisfiable:
			return blob.ErrInvalidRange
		case http.StatusPreconditionFailed:
			return blob.ErrBlobAlreadyExists
		}
	}

	switch {
	case err == nil:
		return nil
	case errors.Is(err, gcsclient.ErrObjectNotExist):
		return blob.ErrBlobNotFound
	default:
		return errors.Wrap(err, "unexpected GCS error")
	}
}

func (gcs *gcsStorage) PutBlob(ctx context.Context, b blob.ID, data blob.Bytes, opts blob.PutOptions) error {
	ctx, cancel := context.WithCancel(ctx)

	obj := gcs.bucket.Object(gcs.getObjectNameString(b))

	conds := gcsclient.Conditions{DoesNotExist: opts.DoNotRecreate}
	if conds != (gcsclient.Conditions{}) {
		obj = obj.If(conds)
	}

	writer := obj.NewWriter(ctx)
	writer.ChunkSize = writerChunkSize
	writer.ContentType = "application/x-kopia"
	writer.Metadata = timestampmeta.ToMap(opts.SetModTime, timeMapKey)

	if opts.RetentionPeriod != 0 {
		retainUntilDate := clock.Now().Add(opts.RetentionPeriod).UTC()
		writer.Retention = &gcsclient.ObjectRetention{
			Mode:        string(blob.Locked),
			RetainUntil: retainUntilDate,
		}
	}

	err := iocopy.JustCopy(writer, data.Reader())
	if err != nil {
		// cancel context before closing the writer causes it to abandon the upload.
		cancel()

		_ = writer.Close() // failing already, ignore the error

		return translateError(err)
	}

	defer cancel()

	// calling close before cancel() causes it to commit the upload.
	if err := writer.Close(); err != nil {
		return translateError(err)
	}

	if opts.GetModTime != nil {
		*opts.GetModTime = writer.Attrs().Updated
	}

	return nil
}

func (gcs *gcsStorage) DeleteBlob(ctx context.Context, b blob.ID) error {
	err := translateError(gcs.bucket.Object(gcs.getObjectNameString(b)).Delete(ctx))
	if errors.Is(err, blob.ErrBlobNotFound) {
		return nil
	}

	return err
}

func (gcs *gcsStorage) ExtendBlobRetention(ctx context.Context, b blob.ID, opts blob.ExtendOptions) error {
	retainUntilDate := clock.Now().Add(opts.RetentionPeriod).UTC().Truncate(time.Second)

	r := &gcsclient.ObjectRetention{
		Mode:        string(blob.Locked),
		RetainUntil: retainUntilDate,
	}

	_, err := gcs.bucket.Object(gcs.getObjectNameString(b)).Update(ctx, gcsclient.ObjectAttrsToUpdate{Retention: r})
	if err != nil {
		return errors.Wrap(err, "unable to extend retention period to "+retainUntilDate.String())
	}

	return nil
}

func (gcs *gcsStorage) getObjectNameString(blobID blob.ID) string {
	return gcs.Prefix + string(blobID)
}

func (gcs *gcsStorage) ListBlobs(ctx context.Context, prefix blob.ID, callback func(blob.Metadata) error) error {
	lst := gcs.bucket.Objects(ctx, &gcsclient.Query{
		Prefix: gcs.getObjectNameString(prefix),
	})

	oa, err := lst.Next()
	for err == nil {
		bm := gcs.getBlobMeta(oa)

		if cberr := callback(bm); cberr != nil {
			return cberr
		}

		oa, err = lst.Next()
	}

	if !errors.Is(err, iterator.Done) {
		return errors.Wrap(err, "ListBlobs")
	}

	return nil
}

func (gcs *gcsStorage) ConnectionInfo() blob.ConnectionInfo {
	return blob.ConnectionInfo{
		Type:   gcsStorageType,
		Config: &gcs.Options,
	}
}

func (gcs *gcsStorage) DisplayName() string {
	return fmt.Sprintf("GCS: %v", gcs.BucketName)
}

func (gcs *gcsStorage) Close(_ context.Context) error {
	return errors.Wrap(gcs.storageClient.Close(), "error closing GCS storage")
}

func (gcs *gcsStorage) toBlobID(blobName string) blob.ID {
	return blob.ID(blobName[len(gcs.Prefix):])
}

// ServiceAccountCredential represents the structure of a Google service account or external account JSON credential file.
type ServiceAccountCredential struct {
	Type                           string            `json:"type"`
	ProjectID                      string            `json:"project_id"`
	PrivateKeyID                   string            `json:"private_key_id"`
	PrivateKey                     string            `json:"private_key"`
	ClientEmail                    string            `json:"client_email"`
	ClientID                       string            `json:"client_id"`
	AuthURI                        string            `json:"auth_uri"`
	TokenURI                       string            `json:"token_uri"`
	TokenURL                       string            `json:"token_url"`
	AuthProviderX509CertURL        string            `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL              string            `json:"client_x509_cert_url"`
	ServiceAccountImpersonationURL string            `json:"service_account_impersonation_url"`
	CredentialSource               *CredentialSource `json:"credential_source"`
}

// CredentialSource represents the credential source for external account credentials.
type CredentialSource struct {
	File                        string            `json:"file"`
	URL                         string            `json:"url"`
	Headers                     map[string]string `json:"headers"`
	EnvironmentID               string            `json:"environment_id"`
	RegionURL                   string            `json:"region_url"`
	RegionalCredVerificationURL string            `json:"regional_cred_verification_url"`
	IMDSv2SessionTokenURL       string            `json:"imdsv2_session_token_url"`
}

// ValidateServiceAccountCredentials validates a service account credential JSON
// according to Google Cloud security requirements for external credentials.
func ValidateServiceAccountCredentials(credJSON []byte) error {
	var cred ServiceAccountCredential

	if err := json.Unmarshal(credJSON, &cred); err != nil {
		return fmt.Errorf("failed to parse credential JSON: %w", err)
	}

	// Validate based on credential type
	switch cred.Type {
	case "service_account":
		// Service account credentials require these fields
		if cred.PrivateKeyID == "" {
			return errors.New("missing required field: private_key_id")
		}

		if cred.PrivateKey == "" {
			return errors.New("missing required field: private_key")
		}

		if cred.ClientEmail == "" {
			return errors.New("missing required field: client_email")
		}

	case "external_account":
		// External account credentials require validation of different fields
		if cred.ServiceAccountImpersonationURL != "" {
			// Validate service_account_impersonation_url format
			if !strings.HasPrefix(cred.ServiceAccountImpersonationURL, "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/") {
				return fmt.Errorf("invalid service_account_impersonation_url: must start with 'https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/', got '%s'", cred.ServiceAccountImpersonationURL) //nolint:err113
			}
		}

		// token_url or token_uri is typically required for external accounts
		if cred.TokenURL == "" && cred.TokenURI == "" {
			return errors.New("missing required field: token_url or token_uri for external_account")
		}

	default:
		// Unknown credential type - this is acceptable as there may be other valid types
		// We don't reject unknown types to maintain compatibility
	}

	return nil
}

// New creates new Google Cloud Storage-backed storage with specified options:
//
// - the 'BucketName' field is required and all other parameters are optional.
//
// By default the connection reuses credentials managed by (https://cloud.google.com/sdk/),
// but this can be disabled by setting IgnoreDefaultCredentials to true.
func New(ctx context.Context, opt *Options, isCreate bool) (blob.Storage, error) {
	_ = isCreate

	if opt.BucketName == "" {
		return nil, errors.New("bucket name must be specified")
	}

	scope := gcsclient.ScopeFullControl
	if opt.ReadOnly {
		scope = gcsclient.ScopeReadOnly
	}

	clientOptions := []option.ClientOption{option.WithScopes(scope)}

	if j := opt.ServiceAccountCredentialJSON; len(j) > 0 {
		// Validate credentials before using them
		if err := ValidateServiceAccountCredentials(j); err != nil {
			return nil, errors.Wrap(err, "invalid service account credentials")
		}

		clientOptions = append(clientOptions, option.WithAuthCredentialsJSON(option.ServiceAccount, j))
	} else if fn := opt.ServiceAccountCredentialsFile; fn != "" {
		// Read and validate file credentials
		credJSON, err := os.ReadFile(fn) //nolint:gosec
		if err != nil {
			return nil, errors.Wrap(err, "failed to read credentials file")
		}

		if err := ValidateServiceAccountCredentials(credJSON); err != nil {
			return nil, errors.Wrap(err, "invalid service account credentials file")
		}

		clientOptions = append(clientOptions, option.WithAuthCredentialsFile(option.ServiceAccount, fn))
	}

	cli, err := gcsclient.NewClient(ctx, clientOptions...)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create GCS client")
	}

	st := &gcsStorage{
		Options:       *opt,
		storageClient: cli,
		bucket:        cli.Bucket(opt.BucketName),
	}

	gcs, err := maybePointInTimeStore(ctx, st, opt.PointInTime)
	if err != nil {
		return nil, err
	}

	// verify GCS connection is functional by listing blobs in a bucket, which will fail if the bucket
	// does not exist. We list with a prefix that will not exist, to avoid iterating through any objects.
	nonExistentPrefix := fmt.Sprintf("kopia-gcs-storage-initializing-%v", clock.Now().UnixNano())

	err = gcs.ListBlobs(ctx, blob.ID(nonExistentPrefix), func(_ blob.Metadata) error {
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list from the bucket")
	}

	return retrying.NewWrapper(gcs), nil
}

func init() {
	blob.AddSupportedStorage(gcsStorageType, Options{}, New)
}
