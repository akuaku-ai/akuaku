package state

import (
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

// NewID builds a unique run identifier from a backend key, a time, and a random
// suffix. The suffix guarantees uniqueness across runs started in the same
// nanosecond.
func NewID(backend string, now time.Time, suffix string) string {
	return fmt.Sprintf("%s-%d-%s", backend, now.UnixNano(), suffix)
}

// RandomSuffix reads four bytes from r and returns them hex-encoded. Callers
// pass crypto/rand.Reader in production; tests inject a deterministic reader.
func RandomSuffix(r io.Reader) (string, error) {
	b := make([]byte, 4)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
