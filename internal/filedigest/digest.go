// Package filedigest computes stable source-file identities.
package filedigest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// SHA256Hex returns an unprefixed SHA-256 digest for path.
func SHA256Hex(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}

// SHA256 returns a prefixed SHA-256 digest for path.
func SHA256(path string) (string, error) {
	digest, err := SHA256Hex(path)
	if err != nil {
		return "", err
	}
	return "sha256:" + digest, nil
}
