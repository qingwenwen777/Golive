package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func notificationHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])[:32]
}
