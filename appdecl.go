package tela

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The declaration of the app (ADR-294): what the packaging commands need to
// know that the running app does not (who published it, what it opens, which
// parts of the system it uses). Run ignores these fields; tela build and
// tela package read them from the source, without running the program, so
// they must be constants: strings, slices and struct literals of constants.

// HandlerRole is what the app does with the files of a [FileType]: the
// role of macOS's CFBundleTypeRole. The names and values are those of the
// desktop package's HandlerRole, which tela cannot import.
type HandlerRole uint8

const (
	// HandlerDefault (the zero value) is HandlerEditor.
	HandlerDefault HandlerRole = iota
	// HandlerEditor: the app opens, edits and saves files of the type.
	HandlerEditor
	// HandlerViewer: the app opens files of the type without editing them.
	HandlerViewer
	// HandlerShell: the app provides runtime services for the type (macOS's
	// Shell role; elsewhere the same as HandlerEditor).
	HandlerShell
	// HandlerNone: the app declares the type, so the desktop knows its name
	// and icon, without opening it: no "Open with" entry, never the
	// default.
	HandlerNone
)

// FileType is a file type the app opens, in [App].FileTypes. The packages
// write it where each system keeps it: the Info.plist's CFBundleDocumentTypes
// on macOS, the MSIX manifest's file type associations on Windows, the
// .desktop file's MimeType and a MIME package on Linux.
//
//	FileTypes: []tela.FileType{
//	    {Ext: ".note", MIME: "application/x-notes-note", Description: "Notes document"},
//	    {Ext: ".notes-backup", MIME: "application/x-notes-backup", Description: "Notes backup", Role: tela.HandlerViewer},
//	},
type FileType struct {
	// Ext is the extension with its dot (".note"): ASCII letters, digits,
	// '-' or '_'.
	Ext string
	// MIME is the type's MIME type ("application/x-notes-note"), without
	// parameters.
	MIME string
	// Description names the type in the user's language ("Notes document").
	Description string
	// Role is what the app does with the files; the zero value is
	// [HandlerEditor].
	Role HandlerRole
}

// Services are the parts of the system the app uses, in [App].Services. The
// packages turn them into what each system asks for: the Info.plist's usage
// descriptions (NS...UsageDescription) and the macOS entitlements, the MSIX
// manifest's capabilities, the flatpak manifest's permissions. What the app
// does not declare stays closed in a sandbox, and on macOS the first use of
// a service without its usage string ends the process, so declare each one
// the app uses.
//
// The first seven fields are the text the system shows when it asks the user
// for permission, in the user's language; empty means the app does not use
// the service. The booleans say the app uses the service.
type Services struct {
	// Camera, Microphone, Location, Contacts, Calendars, Photos and
	// Bluetooth are the usage strings, such as "Take a picture for your
	// profile".
	Camera, Microphone, Location, Contacts, Calendars, Photos, Bluetooth string
	// Network: the app connects to the network (macOS's client network
	// entitlement, the MSIX internetClient capability, flatpak's network
	// access).
	Network bool
	// Notifications: the app shows notifications (desktop.Notify).
	Notifications bool
	// Secrets: the app stores secrets in the system's keychain (desktop's
	// secrets; flatpak's access to the Secret Service).
	Secrets bool
	// GlobalShortcuts: the app registers shortcuts that work while another
	// app has the focus (desktop's global shortcuts).
	GlobalShortcuts bool
	// Files: the app reads and writes files outside the ones the user
	// picks in the system's dialogs (flatpak's home access, MSIX's
	// broadFileSystemAccess, macOS's user-selected files entitlement).
	Files bool
}

// checkDeclaration panics, in dev mode, with the fix for the mistakes of
// the declaration that compile: a scheme with ':', an extension without its
// dot, a file type without a MIME type. It applies the rules of
// platform.CheckAssociation, which the registration of desktop applies too.
func (app App) checkDeclaration() {
	for _, s := range app.Schemes {
		if strings.Contains(s, ":") {
			panic(fmt.Sprintf("tela: App.Schemes: %q has a ':'; give the scheme without it (%q)", s, strings.TrimRight(s, ":/")))
		}
		platform.CheckAssociation("App.Schemes", platform.Association{Scheme: s})
	}
	for i, ft := range app.FileTypes {
		what := fmt.Sprintf("App.FileTypes[%d]", i)
		if ft.Ext != "" && ft.Ext[0] != '.' {
			panic(fmt.Sprintf("tela: %s: the extension %q has no dot; write it as %q", what, ft.Ext, "."+ft.Ext))
		}
		if ft.Ext == "" {
			panic(fmt.Sprintf("tela: %s: Ext is empty; give the extension with its dot, such as \".note\"", what))
		}
		platform.CheckAssociation(what, platform.Association{
			Extension: ft.Ext, MIMEType: ft.MIME, Description: ft.Description, Role: platform.HandlerRole(ft.Role)})
	}
}
