package gcs_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/blobtesting"
	"github.com/kopia/kopia/internal/providervalidation"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/gcs"
)

const (
	testBucketEnv                 = "KOPIA_GCS_TEST_BUCKET"
	testBucketProjectID           = "KOPIA_GCS_TEST_PROJECT_ID"
	testBucketCredentialsJSONGzip = "KOPIA_GCS_CREDENTIALS_JSON_GZIP"
	testImmutableBucketEnv        = "KOPIA_GCS_TEST_IMMUTABLE_BUCKET"
)

func TestCleanupOldData(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)
	ctx := testlogging.Context(t)

	st, err := gcs.New(ctx, mustGetOptionsOrSkip(t, ""), false)
	require.NoError(t, err)

	defer st.Close(ctx)

	blobtesting.CleanupOldData(ctx, t, st, blobtesting.MinCleanupAge)
}

func TestGCSStorage(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	ctx := testlogging.Context(t)

	// use context that gets canceled after opening storage to ensure it's not used beyond New().
	newctx, cancel := context.WithCancel(ctx)
	st, err := gcs.New(newctx, mustGetOptionsOrSkip(t, uuid.NewString()), false)

	cancel()
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx := testlogging.ContextForCleanup(t)

		blobtesting.CleanupOldData(ctx, t, st, 0)
		st.Close(ctx)
	})

	blobtesting.VerifyStorage(ctx, t, st, blob.PutOptions{})

	blobtesting.AssertConnectionInfoRoundTrips(ctx, t, st)
	require.NoError(t, providervalidation.ValidateProvider(ctx, st, blobtesting.TestValidationOptions))
}

func TestGCSStorageInvalid(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	bucket := getEnvVarOrSkip(t, testBucketEnv)

	ctx := testlogging.Context(t)

	_, err := gcs.New(ctx, &gcs.Options{
		BucketName:                   bucket + "-no-such-bucket",
		ServiceAccountCredentialJSON: getCredJSONFromEnv(t),
	}, false)
	require.Error(t, err, "unexpected success connecting to GCS, wanted error")
}

func gunzip(d []byte) ([]byte, error) {
	z, err := gzip.NewReader(bytes.NewReader(d))
	if err != nil {
		return nil, err
	}

	defer z.Close()

	return io.ReadAll(z)
}

func getEnvVarOrSkip(t *testing.T, envVarName string) string {
	t.Helper()

	v := os.Getenv(envVarName)
	if v == "" {
		t.Skipf("%q is not set", envVarName)
	}

	return v
}

func getCredJSONFromEnv(t *testing.T) []byte {
	t.Helper()

	b64Data := getEnvVarOrSkip(t, testBucketCredentialsJSONGzip)

	credDataGZ, err := base64.StdEncoding.DecodeString(b64Data)
	require.NoError(t, err, "GCS credentials env value can't be decoded")

	credJSON, err := gunzip(credDataGZ)
	require.NoError(t, err, "GCS credentials env can't be unzipped")

	return credJSON
}

func mustGetOptionsOrSkip(t *testing.T, prefix string) *gcs.Options {
	t.Helper()

	bucket := getEnvVarOrSkip(t, testBucketEnv)

	return &gcs.Options{
		BucketName:                   bucket,
		ServiceAccountCredentialJSON: getCredJSONFromEnv(t),
		Prefix:                       prefix,
	}
}

func getBlobCount(ctx context.Context, t *testing.T, st blob.Storage, prefix blob.ID) int {
	t.Helper()

	var count int

	err := st.ListBlobs(ctx, prefix, func(bm blob.Metadata) error {
		count++
		return nil
	})
	require.NoError(t, err)

	return count
}

func TestValidateServiceAccountCredentials(t *testing.T) {
	t.Parallel()

	validServiceAccountCred := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "key123",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	validExternalAccountCred := `{
		"type": "external_account",
		"audience": "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
		"subject_token_type": "urn:ietf:params:aws:token-type:aws4_request",
		"token_url": "https://sts.googleapis.com/v1/token",
		"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
		"credential_source": {
			"environment_id": "aws1",
			"region_url": "http://169.254.169.254/latest/meta-data/placement/region",
			"url": "http://169.254.169.254/latest/meta-data/iam/security-credentials",
			"regional_cred_verification_url": "https://sts.{region}.amazonaws.com?Action=GetCallerIdentity&Version=2011-06-15"
		}
	}`

	tests := []struct {
		name    string
		json    string
		wantErr bool
		errText string
	}{
		{
			name:    "valid service account credentials",
			json:    validServiceAccountCred,
			wantErr: false,
		},
		{
			name:    "valid external account credentials",
			json:    validExternalAccountCred,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			json:    `not json`,
			wantErr: true,
			errText: "failed to parse credential JSON",
		},
		{
			name:    "unknown credential type is accepted",
			json:    `{"type": "authorized_user"}`,
			wantErr: false,
		},
		{
			name:    "service account missing private_key_id",
			json:    `{"type": "service_account", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n", "client_email": "test@test.iam.gserviceaccount.com"}`,
			wantErr: true,
			errText: "missing required field: private_key_id",
		},
		{
			name:    "service account missing private_key",
			json:    `{"type": "service_account", "private_key_id": "key123", "client_email": "test@test.iam.gserviceaccount.com"}`,
			wantErr: true,
			errText: "missing required field: private_key",
		},
		{
			name:    "service account missing client_email",
			json:    `{"type": "service_account", "private_key_id": "key123", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n"}`,
			wantErr: true,
			errText: "missing required field: client_email",
		},
		{
			name:    "external account missing token_url",
			json:    `{"type": "external_account", "audience": "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider"}`,
			wantErr: true,
			errText: "missing required field: token_url or token_uri",
		},
		{
			name:    "external account with invalid service_account_impersonation_url",
			json:    `{"type": "external_account", "token_url": "https://sts.googleapis.com/v1/token", "service_account_impersonation_url": "https://evil.com/impersonate"}`,
			wantErr: true,
			errText: "invalid service_account_impersonation_url",
		},
		{
			name:    "external account with credential_source file",
			json:    `{"type": "external_account", "token_url": "https://sts.googleapis.com/v1/token", "credential_source": {"file": "/path/to/token"}}`,
			wantErr: false,
		},
		{
			name: "external account with credential_source aws",
			json: `{"type": "external_account", "token_url": "https://sts.googleapis.com/v1/token", "credential_source": {` +
				`"url": "http://169.254.169.254/latest/meta-data/iam/security-credentials", ` +
				`"region_url": "http://169.254.169.254/latest/meta-data/placement/region", ` +
				`"imdsv2_session_token_url": "http://169.254.169.254/latest/api/token"}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := gcs.ValidateServiceAccountCredentials([]byte(tt.json))
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errText)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
