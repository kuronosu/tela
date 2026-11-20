package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// randomString returns n random bytes from crypto/rand, base64url
// encoded without padding. A code verifier of 32 bytes is 43 characters
// (RFC 7636 §4.1).
func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on the supported systems; if it
		// does, nothing safe is left to do.
		panic("oauth: no random numbers: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// challengeS256 returns the S256 code challenge of verifier (RFC 7636
// §4.2): BASE64URL(SHA256(verifier)).
func challengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
