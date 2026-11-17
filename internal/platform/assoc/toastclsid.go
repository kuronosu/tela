package assoc

import (
	"crypto/sha1"
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
)

// ToastNamespace is the namespace of the name-based UUIDs that identify
// the toast activators of Tela apps on Windows: 55af4951-5c8d-5014-a922-
// c4a2225bd6cc, itself the version 5 UUID of the URL
// "https://telaui.dev/ns/toast-activator" in RFC 4122's URL namespace
// (6ba7b811-9dad-11d1-80b4-00c04fd430c8). It never changes: every CLSID
// [ToastActivatorCLSID] returned depends on it.
var ToastNamespace = [16]byte{0x55, 0xaf, 0x49, 0x51, 0x5c, 0x8d, 0x50, 0x14, 0xa9, 0x22, 0xc4, 0xa2, 0x22, 0x5b, 0xd6, 0xcc}

// ToastActivatorFlag is the command-line flag of a process COM starts to
// deliver a toast's click (the convention of the Windows toast
// documentation, "-ToastActivated"): the LocalServer32 command of the
// unpackaged registration, and the com:ExeServer Arguments of an MSIX
// manifest. COM may append "-Embedding" after it.
const ToastActivatorFlag = "-ToastActivated"

// ToastActivatorCLSID returns the CLSID of the COM toast activator of the
// app appID on Windows (ADR-297), in the registry's and MSIX manifests'
// text form without braces, upper case: "620BE956-74D9-56B1-BE2A-
// 2D118CB430C5" for "com.example.Notes". It is the version 5 UUID (RFC
// 4122, SHA-1) of the UTF-8 app ID in [ToastNamespace], so the same app ID
// gives the same CLSID in every version of the app and on every machine.
// The app ID is the one the running app gives the OS services
// (desktop.AppID: platform.ServiceAppID of SetAppID's, else tela.App's,
// else the executable's name); appID is passed through ServiceAppID too,
// so an ID the services would change gives the CLSID the app registers.
// Case matters: "com.example.Notes" and "com.example.notes" are two apps.
//
// The Win32 backend registers its class object under this CLSID, and
// writes it as the AUMID's CustomActivator; packaging writes it as an MSIX
// manifest's ToastActivatorCLSID and com:Class Id.
func ToastActivatorCLSID(appID string) string {
	return nameUUID(ToastNamespace, platform.ServiceAppID(appID))
}

// nameUUID returns the version 5 UUID of name in namespace ns, upper case.
func nameUUID(ns [16]byte, name string) string {
	h := sha1.Sum(append(ns[:], name...))
	h[6] = h[6]&0x0F | 0x50 // version 5
	h[8] = h[8]&0x3F | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%X-%X-%X-%X-%X", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}
