package deviceprint

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint returns a stable hex digest of userAgent and client IP.
func Fingerprint(userAgent, clientIP string) string {
	h := sha256.Sum256([]byte(userAgent + "|" + clientIP))
	return hex.EncodeToString(h[:])
}
