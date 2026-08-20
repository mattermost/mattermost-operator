package utils

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/pkg/errors"
)

// RandomHex returns n random bytes encoded as a hex string (2*n characters).
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, "failed to read random bytes")
	}
	return hex.EncodeToString(b), nil
}
