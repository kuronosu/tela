//go:build darwin

package darwin

import "sync"

// The Security framework's keychain items (SecItem.h), for the secrets of
// the cocoa backend. Opened on first use by LoadSecurity, not by Load: a
// backend without it still works and reports the secrets as unsupported.
// The functions are thread-safe and may block (the keychain's unlock or
// access prompt); dictionaries are CFDictionaryRef, toll-free bridged with
// NSDictionary, so callers build them as NSMutableDictionary.

const frameworkSecurity = "/System/Library/Frameworks/Security.framework/Security"

// SecItem functions, bound by LoadSecurity. They return an OSStatus (0:
// errSecSuccess). A result is +1: CFRelease it.
var (
	SecItemAdd          func(attributes uintptr, result *uintptr) int32
	SecItemCopyMatching func(query uintptr, result *uintptr) int32
	SecItemUpdate       func(query, attributesToUpdate uintptr) int32
	SecItemDelete       func(query uintptr) int32
)

// Keys and values of SecItem dictionaries (CFStringRef constants), set by
// LoadSecurity.
var (
	SecClass                ID // kSecClass
	SecClassGenericPassword ID // kSecClassGenericPassword
	SecAttrService          ID // kSecAttrService
	SecAttrAccount          ID // kSecAttrAccount
	SecAttrLabel            ID // kSecAttrLabel
	SecValueData            ID // kSecValueData
	SecReturnData           ID // kSecReturnData
	SecMatchLimit           ID // kSecMatchLimit
	SecMatchLimitOne        ID // kSecMatchLimitOne
)

var (
	secOnce sync.Once
	secErr  error
)

// LoadSecurity opens the Security framework and binds the SecItem
// functions and constants, once; an error means they are unavailable.
// Safe from any goroutine.
func LoadSecurity() error {
	secOnce.Do(func() {
		secErr = loadOptional(frameworkSecurity, func(lib uintptr) {
			bind(&SecItemAdd, lib, "SecItemAdd")
			bind(&SecItemCopyMatching, lib, "SecItemCopyMatching")
			bind(&SecItemUpdate, lib, "SecItemUpdate")
			bind(&SecItemDelete, lib, "SecItemDelete")
			for _, c := range []struct {
				v    *ID
				name string
			}{
				{&SecClass, "kSecClass"},
				{&SecClassGenericPassword, "kSecClassGenericPassword"},
				{&SecAttrService, "kSecAttrService"},
				{&SecAttrAccount, "kSecAttrAccount"},
				{&SecAttrLabel, "kSecAttrLabel"},
				{&SecValueData, "kSecValueData"},
				{&SecReturnData, "kSecReturnData"},
				{&SecMatchLimit, "kSecMatchLimit"},
				{&SecMatchLimitOne, "kSecMatchLimitOne"},
			} {
				*c.v = ID(symbolValue(lib, c.name))
			}
		})
	})
	return secErr
}
