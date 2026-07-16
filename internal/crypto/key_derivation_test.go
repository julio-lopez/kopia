package crypto_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/crypto"
)

func TestDeriveKeyFromMasterKey(t *testing.T) {
	const testPurpose = "the-test-purpose"

	var (
		testMasterKey = []byte("ABCDEFGHIJKLMNOP")
		testSalt      = []byte("0123456789012345")
	)

	t.Run("ReturnsKey", func(t *testing.T) {
		key, err := crypto.DeriveKeyFromMasterKey(testMasterKey, testSalt, testPurpose, 32)
		require.NoError(t, err)

		expected := "828769ee8969bc37f11dbaa32838f8db6c19daa6e3ae5f5eed2da2d94d8faddb"
		got := fmt.Sprintf("%02x", key)
		require.Equal(t, expected, got)
	})

	t.Run("ErrorOnNilMasterKey", func(t *testing.T) {
		k, err := crypto.DeriveKeyFromMasterKey(nil, testSalt, testPurpose, 32)
		require.Error(t, err)
		require.Nil(t, k)
	})

	t.Run("ErrorOnEmptyMasterKey", func(t *testing.T) {
		k, err := crypto.DeriveKeyFromMasterKey([]byte{}, testSalt, testPurpose, 32)
		require.Error(t, err)
		require.Nil(t, k)
	})
}

func TestValidateKeyDerivationParameters(t *testing.T) {
	tests := []struct {
		name                      string
		inputSecret               []byte
		minInputSecretLength      int
		requestedDerivedKeyLength int
		expectError               bool
	}{
		// Valid cases
		{
			name:                      "ValidWithMinimumValues",
			inputSecret:               make([]byte, 32),
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 16,
			expectError:               false,
		},
		{
			name:                      "ValidWithLargerValues",
			inputSecret:               make([]byte, 64),
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 32,
			expectError:               false,
		},

		// minInputSecretLength validation (internal parameter)
		{
			name:                      "ErrorWhenMinInputSecretLengthBelowMinimum",
			inputSecret:               make([]byte, 32),
			minInputSecretLength:      31,
			requestedDerivedKeyLength: 16,
			expectError:               true,
		},
		{
			name:                      "ErrorWhenMinInputSecretLengthZero",
			inputSecret:               make([]byte, 32),
			minInputSecretLength:      0,
			requestedDerivedKeyLength: 16,
			expectError:               true,
		},

		// inputSecret validation
		{
			name:                      "ErrorWhenInputSecretEmpty",
			inputSecret:               []byte{},
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 16,
			expectError:               true,
		},
		{
			name:                      "ErrorWhenInputSecretNil",
			inputSecret:               nil,
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 16,
			expectError:               true,
		},
		{
			name:                      "ErrorWhenInputSecretTooShort",
			inputSecret:               make([]byte, 16),
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 16,
			expectError:               true,
		},
		{
			name:                      "ErrorWhenInputSecretTooShort_BelowMinInputSecretLength",
			inputSecret:               make([]byte, 32),
			minInputSecretLength:      48,
			requestedDerivedKeyLength: 16,
			expectError:               true,
		},

		// requestedDerivedKeyLength validation
		{
			name:                      "ErrorWhenRequestedDerivedKeyLengthBelowMinimum",
			inputSecret:               make([]byte, 32),
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 15,
			expectError:               true,
		},
		{
			name:                      "ErrorWhenRequestedDerivedKeyLengthZero",
			inputSecret:               make([]byte, 32),
			minInputSecretLength:      32,
			requestedDerivedKeyLength: 0,
			expectError:               true,
		},

		// Edge cases: multiple validation failures
		// When multiple parameters are invalid, first validation error should be returned
		{
			name:                      "ErrorOnMinInputSecretLengthFirst",
			inputSecret:               []byte{},
			minInputSecretLength:      0,
			requestedDerivedKeyLength: 0,
			expectError:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crypto.ValidateKeyDerivationParameters(
				tt.inputSecret,
				tt.minInputSecretLength,
				tt.requestedDerivedKeyLength,
			)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
