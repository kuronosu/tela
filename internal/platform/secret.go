package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode"
)

// MaxSecretSize is the largest secret, in bytes, every backend stores: the
// limit of Windows Credential Manager (CRED_MAX_CREDENTIAL_BLOB_SIZE, 5 *
// 512). Keep keys and tokens in secrets, and encrypt larger data with a key
// kept there.
const MaxSecretSize = 2560

// ErrSecretNotFound is returned by [Desktop.Secret] when no secret has the
// name. errors.Is(err, fs.ErrNotExist) also holds.
var ErrSecretNotFound = fmt.Errorf("platform: secret not found: %w", fs.ErrNotExist)

// ErrSecretCanceled is returned by the secret methods of [Desktop] when the
// user dismissed the OS's unlock prompt (the Secret Service's keyring
// password): nothing was read or written. Win32 has no prompt.
var ErrSecretCanceled = errors.New("platform: the user canceled the keyring's unlock prompt; the secret was not read or written")

// ErrSecretTooLarge is returned by [Desktop.SetSecret] for a secret larger
// than MaxSecretSize.
var ErrSecretTooLarge = errors.New("platform: secret larger than MaxSecretSize (2560 bytes)")

// CheckSecretName panics with an actionable message if appID is not a
// valid app ID (CheckAppID) or name cannot name a secret: empty, longer
// than 255 bytes, or holding control characters. what names the call.
func CheckSecretName(what, appID, name string) {
	CheckAppID(what, appID)
	if name == "" || len(name) > 255 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		panicf("platform: %s: invalid secret name %q; it must be non-empty text without control characters, at most 255 bytes", what, name)
	}
}

// CheckSecret checks the arguments of SetSecret: it panics for an invalid
// name (CheckSecretName) and returns ErrSecretTooLarge for a value above
// MaxSecretSize, so every backend refuses the same secrets.
func CheckSecret(appID, name string, secret []byte) error {
	CheckSecretName("SetSecret", appID, name)
	if len(secret) > MaxSecretSize {
		return fmt.Errorf("platform: SetSecret(%q, %q): %d bytes: %w", appID, name, len(secret), ErrSecretTooLarge)
	}
	return nil
}
