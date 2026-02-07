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

	validCred := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "key123",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	tests := []struct {
		name    string
		json    string
		wantErr bool
		errText string
	}{
		{
			name:    "valid credentials",
			json:    validCred,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			json:    `not json`,
			wantErr: true,
			errText: "failed to parse credential JSON",
		},
		{
			name:    "wrong type",
			json:    `{"type": "authorized_user"}`,
			wantErr: true,
			errText: "invalid credential type",
		},
		{
			name:    "missing private_key_id",
			json:    `{"type": "service_account", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n", "client_email": "test@test.iam.gserviceaccount.com", "client_id": "123"}`,
			wantErr: true,
			errText: "missing required field: private_key_id",
		},
		{
			name:    "missing private_key",
			json:    `{"type": "service_account", "private_key_id": "key123", "client_email": "test@test.iam.gserviceaccount.com", "client_id": "123"}`,
			wantErr: true,
			errText: "missing required field: private_key",
		},
		{
			name:    "missing client_email",
			json:    `{"type": "service_account", "private_key_id": "key123", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n", "client_id": "123"}`,
			wantErr: true,
			errText: "missing required field: client_email",
		},
		{
			name:    "missing client_id",
			json:    `{"type": "service_account", "private_key_id": "key123", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n", "client_email": "test@test.iam.gserviceaccount.com"}`,
			wantErr: true,
			errText: "missing required field: client_id",
		},
		{
			name:    "invalid client_email format",
			json:    `{"type": "service_account", "private_key_id": "key123", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n", "client_email": "test@example.com", "client_id": "123"}`,
			wantErr: true,
			errText: "invalid client_email format",
		},
		{
			name:    "invalid private_key format",
			json:    `{"type": "service_account", "private_key_id": "key123", "private_key": "not a key", "client_email": "test@test.iam.gserviceaccount.com", "client_id": "123"}`,
			wantErr: true,
			errText: "invalid private_key format",
		},
		{
			name: "invalid token_uri",
			json: `{"type": "service_account", "private_key_id": "key123", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n` +
				`-----END PRIVATE KEY-----\n", "client_email": "test@test.iam.gserviceaccount.com", "client_id": "123", ` +
				`"token_uri": "https://evil.com/token"}`,
			wantErr: true,
			errText: "invalid token_uri",
		},
		{
			name: "invalid auth_uri",
			json: `{"type": "service_account", "private_key_id": "key123", "private_key": "-----BEGIN PRIVATE KEY-----\ntest\n` +
				`-----END PRIVATE KEY-----\n", "client_email": "test@test.iam.gserviceaccount.com", "client_id": "123", ` +
				`"auth_uri": "https://evil.com/auth"}`,
			wantErr: true,
			errText: "invalid auth_uri",
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
