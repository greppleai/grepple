package router

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func verifyWebhook(secret string, raw []byte, signature string) bool {
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write(raw)
	expected := "sha256=" + hex.EncodeToString(hash.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
