package sessionx

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// idBytes is the entropy behind a session id. 32 bytes is 256 bits, which
// puts a guessing attack out of reach for any online adversary.
const idBytes = 32

// NewID returns a fresh session identifier: 32 crypto/rand bytes in
// unpadded base64url, safe in a cookie, a URL and a database key.
//
// It returns an error rather than panicking when the system entropy source
// fails, because a process that cannot generate a secure id must refuse the
// login, not continue with a weak one.
func NewID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// EqualID compares two ids in constant time. Length is compared first and
// leaks only the length, which is fixed for ids this package issues.
func EqualID(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
