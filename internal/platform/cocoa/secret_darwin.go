//go:build darwin

package cocoa

import (
	"fmt"
	"runtime"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Secrets live in the user's keychain as generic passwords (ADR-224):
// kSecClassGenericPassword items whose service (kSecAttrService) is the
// app ID and whose account (kSecAttrAccount) is the secret's name, the
// value in kSecValueData. Keychain Access shows them under the app ID.
//
// The calls are SecItemAdd (then SecItemUpdate when the item exists),
// SecItemCopyMatching and SecItemDelete. They block while the keychain
// asks the user (to unlock it, or to let this program read an item
// another program created) and are safe from any goroutine: each call
// locks its goroutine to its thread for the length of its autorelease
// pool. The file-based login keychain is used, which needs no
// entitlement; the data protection keychain (kSecUseDataProtectionKeychain)
// needs a signed app with a keychain access group, which go build does not
// make. services.go maps the OSStatus codes.

// SetSecret stores the secret, replacing an earlier value.
func (desktop) SetSecret(appID, name string, secret []byte) error {
	if err := platform.CheckSecret(appID, name, secret); err != nil {
		return err
	}
	return withKeychain("SetSecret", func() error {
		query := secretQuery(appID, name)
		data := darwin.NSData(secret)
		defer darwin.Release(data)
		attrs := secretQuery(appID, name)
		darwin.DictionarySet(attrs, darwin.SecValueData, data)
		status := darwin.SecItemAdd(uintptr(attrs), nil)
		if status == errSecDuplicateItem {
			update := darwin.NewDictionary()
			darwin.DictionarySet(update, darwin.SecValueData, data)
			status = darwin.SecItemUpdate(uintptr(query), uintptr(update))
		}
		return secretStatusError(fmt.Sprintf("SetSecret(%q, %q)", appID, name), status)
	})
}

// Secret reads the secret, or ErrSecretNotFound.
func (desktop) Secret(appID, name string) ([]byte, error) {
	platform.CheckSecretName("Secret", appID, name)
	var out []byte
	err := withKeychain("Secret", func() error {
		query := secretQuery(appID, name)
		darwin.DictionarySet(query, darwin.SecReturnData, darwin.NSBool(true))
		darwin.DictionarySet(query, darwin.SecMatchLimit, darwin.SecMatchLimitOne)
		result := new(uintptr) // on the heap, which does not move during the call
		status := darwin.SecItemCopyMatching(uintptr(query), result)
		if err := secretStatusError(fmt.Sprintf("Secret(%q, %q)", appID, name), status); err != nil {
			return err
		}
		if *result == 0 {
			return fmt.Errorf("cocoa: Secret(%q, %q): the keychain returned no data", appID, name)
		}
		out = darwin.Bytes(darwin.ID(*result))
		darwin.CFRelease(*result)
		if out == nil {
			out = []byte{}
		}
		return nil
	})
	return out, err
}

// DeleteSecret removes the secret; a missing one is not an error.
func (desktop) DeleteSecret(appID, name string) error {
	platform.CheckSecretName("DeleteSecret", appID, name)
	return withKeychain("DeleteSecret", func() error {
		status := darwin.SecItemDelete(uintptr(secretQuery(appID, name)))
		if status == errSecItemNotFound {
			return nil
		}
		return secretStatusError(fmt.Sprintf("DeleteSecret(%q, %q)", appID, name), status)
	})
}

// secretQuery returns an autoreleased dictionary naming the item of a
// secret.
func secretQuery(appID, name string) darwin.ID {
	q := darwin.NewDictionary()
	darwin.DictionarySet(q, darwin.SecClass, darwin.SecClassGenericPassword)
	darwin.DictionarySet(q, darwin.SecAttrService, darwin.AutoString(appID))
	darwin.DictionarySet(q, darwin.SecAttrAccount, darwin.AutoString(name))
	return q
}

// withKeychain runs fn, on any goroutine, with the Security framework
// loaded, its thread locked and an autorelease pool around it.
func withKeychain(what string, fn func() error) error {
	if err := darwin.LoadSecurity(); err != nil {
		return fmt.Errorf("cocoa: %s: the Security framework is missing: %w (%w)", what, platform.ErrUnsupported, err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	return fn()
}
