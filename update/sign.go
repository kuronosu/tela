package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PublicKey is an ed25519 public key that may sign the manifest, in the
// text form tela update keygen prints: "ed25519:" and the standard base64
// of the 32-byte key.
//
//	update.Config{
//	    URL:        "https://example.com/notes/manifest.json",
//	    PublicKeys: []update.PublicKey{"ed25519:Wp9H1N3dD6c0m7v7kxq8nBzWJm1eJ3c0c8wzJmq7oJ0="},
//	}
//
// Its key ID ([PublicKey.ID]) is derived from the key, so a signature
// file names which key made each signature without a separate list.
type PublicKey string

const publicKeyPrefix = "ed25519:"

// NewPublicKey returns the text form of k.
func NewPublicKey(k ed25519.PublicKey) PublicKey {
	return PublicKey(publicKeyPrefix + base64.StdEncoding.EncodeToString(k))
}

// Key returns the key, or an error when k is not "ed25519:" and the base64
// of 32 bytes.
func (k PublicKey) Key() (ed25519.PublicKey, error) {
	b64, ok := strings.CutPrefix(string(k), publicKeyPrefix)
	if !ok {
		return nil, fmt.Errorf("update: public key %q does not start with %q", string(k), publicKeyPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("update: public key %q is not the base64 of %d bytes", string(k), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// ID returns the key ID of k ([KeyID]), "" when k is not a valid key.
func (k PublicKey) ID() string {
	key, err := k.Key()
	if err != nil {
		return ""
	}
	return KeyID(key)
}

// KeyID returns the ID of a public key as signature files name it: the
// first 8 bytes of the SHA-256 of the 32-byte key, in lowercase hex (16
// characters).
func KeyID(k ed25519.PublicKey) string {
	sum := sha256.Sum256(k)
	return hex.EncodeToString(sum[:8])
}

// SignatureFile is the detached signature of a manifest, the JSON file
// <manifest>.sig:
//
//	{
//	  "signatures": [
//	    {"keyID": "3f9a0c21d4e5b6a7", "sig": "base64 of the 64-byte ed25519 signature"}
//	  ]
//	}
//
// Each signature is ed25519 (RFC 8032, not prehashed) over the manifest's
// exact bytes. A client accepts the manifest when one signature by a key
// of its Config.PublicKeys verifies, and ignores signatures by keys it
// does not know: to rotate keys, sign with the old and the new key, ship
// the new key in a release, and later sign with the new one only.
type SignatureFile struct {
	Signatures []Signature `json:"signatures"`
}

// Signature is one signature of a [SignatureFile].
type Signature struct {
	KeyID string `json:"keyID"`
	Sig   string `json:"sig"`
}

// Sign returns the signature file of manifest signed by each key, the
// bytes tela update sign writes to <manifest>.sig.
func Sign(manifest []byte, keys ...ed25519.PrivateKey) ([]byte, error) {
	if len(keys) == 0 {
		return nil, errors.New("update.Sign: no key")
	}
	var f SignatureFile
	for _, k := range keys {
		if len(k) != ed25519.PrivateKeySize {
			return nil, errors.New("update.Sign: not an ed25519 private key")
		}
		f.Signatures = append(f.Signatures, Signature{
			KeyID: KeyID(k.Public().(ed25519.PublicKey)),
			Sig:   base64.StdEncoding.EncodeToString(ed25519.Sign(k, manifest)),
		})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Verify checks that sig, the bytes of a [SignatureFile], holds a valid
// signature of manifest by one of keys. The error matches [ErrSignature].
// Check calls it before it reads anything in the manifest.
func Verify(manifest, sig []byte, keys []PublicKey) error {
	if len(keys) == 0 {
		return fmt.Errorf("%w: Config.PublicKeys is empty", ErrSignature)
	}
	trusted := make(map[string]ed25519.PublicKey, len(keys))
	for _, k := range keys {
		key, err := k.Key()
		if err != nil {
			return fmt.Errorf("%w: Config.PublicKeys: %w", ErrSignature, err)
		}
		trusted[KeyID(key)] = key
	}
	var f SignatureFile
	if err := json.Unmarshal(sig, &f); err != nil {
		return fmt.Errorf("%w: the signature file is not valid JSON: %v", ErrSignature, err)
	}
	var unknown []string
	for _, s := range f.Signatures {
		key, ok := trusted[s.KeyID]
		if !ok {
			unknown = append(unknown, s.KeyID)
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(s.Sig)
		if err == nil && ed25519.Verify(key, manifest, raw) {
			return nil
		}
	}
	if len(f.Signatures) == 0 {
		return fmt.Errorf("%w: the signature file has no signature", ErrSignature)
	}
	if len(unknown) == len(f.Signatures) {
		return fmt.Errorf("%w: signed only by keys this app does not trust (key IDs %s)", ErrSignature, strings.Join(unknown, ", "))
	}
	return fmt.Errorf("%w: the signature does not match the manifest", ErrSignature)
}
