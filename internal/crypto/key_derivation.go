package crypto

import (
	"crypto/hkdf"
	"crypto/sha256"

	"github.com/pkg/errors"
)

const (
	minDerivedKeyLength = 16
	minInputKeyLength   = 32
)

var (
	errInvalidMasterKey          = errors.New("invalid primary key")
	errInvalidKeyLengthParameter = errors.New("invalid key length parameter")
)

// DeriveKeyFromMasterKey computes a key for a specific purpose and length using HKDF based on the master key.
func DeriveKeyFromMasterKey(masterKey, salt []byte, purpose string, length int) (derivedKey []byte, err error) {
	if len(masterKey) == 0 {
		return nil, errors.Wrap(errInvalidMasterKey, "empty key")
	}

	if derivedKey, err = hkdf.Key(sha256.New, masterKey, salt, purpose, length); err != nil {
		return nil, errors.Wrap(err, "unable to derive key")
	}

	return derivedKey, nil
}

// ValidateKeyDerivationParameters ensures the parameters used for key derivation
// meet the minimum requirements.
func ValidateKeyDerivationParameters(inputSecret []byte, minInputSecretLength, requestedDerivedKeyLength int) error {
	if minInputSecretLength < minInputKeyLength {
		// this should be treated as a programming error since minInputSecretLength is an internal
		// value and not an external input; this could be replaced with a panic
		return errors.Wrapf(errInvalidKeyLengthParameter, "requested minimum input key length is below the minimum allowed (%v < %v)", minInputSecretLength, minInputKeyLength)
	}

	if requestedDerivedKeyLength < minDerivedKeyLength {
		// this should be treated as a programming error and could be replaced with a panic
		return errors.Wrapf(errInvalidKeyLengthParameter, "requested length for derived key is too short (%v bytes), it should be at least %v bytes", requestedDerivedKeyLength, minDerivedKeyLength)
	}

	if isl := len(inputSecret); isl == 0 {
		return errors.Wrap(errInvalidMasterKey, "empty key")
	} else if isl < minInputSecretLength {
		return errors.Wrapf(errInvalidMasterKey, "input key used for derivation is too short (%v bytes), it should be at least %v bytes long", isl, minInputSecretLength)
	}

	return nil
}
