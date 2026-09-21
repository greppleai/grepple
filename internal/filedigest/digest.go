// Package filedigest computes stable source-file identities.
package filedigest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// SHA256 returns a prefixed SHA-256 digest for path.
func SHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
