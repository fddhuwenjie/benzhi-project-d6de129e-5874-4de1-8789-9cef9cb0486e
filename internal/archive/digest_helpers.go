package archive

import "crypto/sha256"

// DigestBytes computes the canonical SHA-256 digest used by archive records.
func DigestBytes(payload []byte) [32]byte { return sha256.Sum256(payload) }
