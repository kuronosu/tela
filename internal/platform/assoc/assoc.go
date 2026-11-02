// Package assoc registers an app as a handler of URL schemes and file
// types for the current user: the OS side of platform.Associations (see
// "Registration of schemes and file types" in package platform, ADR-209,
// ADR-210). It works without a running app or backend, so installers
// (packaging) call [Native] directly; a running app reaches the same
// implementation through platform.Desktop.Associations, which the headless
// backend answers with a [Fake].
//
// # Where things are written
//
//   - Linux ([Linux]): the app's own desktop entry
//     $XDG_DATA_HOME/applications/<ID>.desktop, with MimeType= listing
//     x-scheme-handler/<scheme> and the file types' MIME types, marked as
//     written by Tela (X-Tela-Registered=true) so Register knows its own
//     file; for an extension that no MIME type claims yet, a MIME package
//     $XDG_DATA_HOME/mime/packages/<ID>.xml. After a change it rebuilds
//     the directory's mimeinfo.cache itself (GIO and xdg-mime read
//     handlers from it) and runs update-mime-database when it is
//     installed (LinuxDirs.Run; optional, see linux.go). It never writes
//     mimeapps.list except for MakeDefault (the [Default Applications]
//     group of $XDG_CONFIG_HOME/mimeapps.list) and Unregister (removing
//     the defaults that name the app's desktop entry). Status reads the
//     mimeapps.list files and desktop entries in the XDG search order
//     (the user's, then DataDirs/ConfigDirs, with
//     <desktop>-mimeapps.list first), as GIO does. What counts as
//     another application's association (RegisterOptions.Replace): a
//     desktop entry of the app's ID that Tela did not write (the user's,
//     or a system one the user's would shadow) and does not list the
//     type yet, and an extension that another MIME type claims. Other
//     applications' defaults and desktop entries listing the type are
//     never touched: they are reported by Status (Other).
//   - Windows ([Windows]; windows.go has the details): HKCU only, so no
//     administrator rights; HKLM is read, never written (a per-user key
//     shadows the machine's, so what is registered there counts as another
//     application's). A scheme is the key Classes\<scheme> ("URL
//     Protocol" value, shell\open\command, and values that say which app
//     and executable wrote it); a file type is the ProgID
//     Classes\<ID>.<ext without dot> (with its command and icon) added to
//     Classes\.<ext>\OpenWithProgids; the extension's default value (the
//     fallback handler) is set only when nothing names one, and replaced
//     only with Replace. The app is listed in Software\<ID>\Capabilities
//     (URLAssociations, FileAssociations) and
//     Software\RegisteredApplications for the Default apps page. After a
//     change it calls SHChangeNotify (SHCNE_ASSOCCHANGED). Status reads the
//     user's choice from the UserChoice keys; MakeDefault returns DefaultSet
//     when the app already is the default (a scheme nothing overrides, an
//     extension nothing else opens, or the user's choice), and otherwise
//     opens ms-settings:defaultapps (DefaultAsked), because Windows lets
//     only the user choose.
//   - macOS ([MacOS]; macos.go has the details, ADR-228): the app is its
//     bundle, so HandlerApp.Exec (else the running executable) must be
//     inside <Name>.app/Contents/MacOS/ and the app ID must be the
//     bundle's CFBundleIdentifier; otherwise every call fails (outside a
//     bundle with ErrUnsupported, naming `tela build` and `tela run`). A
//     scheme is an entry of the bundle's Info.plist CFBundleURLTypes; a
//     file type an entry of CFBundleDocumentTypes for the extension's
//     content type (LSHandlerRank Alternate, CFBundleTypeRole Editor),
//     plus a UTImportedTypeDeclarations entry when nothing declares the
//     extension. Entries are marked TelaRegistered=true, other keys and
//     entries kept, the file rewritten only when an entry is added or
//     removed (never in a signed bundle). Then LSRegisterURL makes Launch
//     Services read the bundle again. Launch Services keeps every app's
//     claims side by side, so nothing is ever taken (Replace changes
//     nothing), and it makes the only candidate of a scheme or type its
//     handler by itself. MakeDefault calls LSSetDefaultHandlerForURLScheme
//     or LSSetDefaultRoleHandlerForContentType; Status reads the
//     Info.plist and Launch Services' handlers and default (Other: a
//     bundle ID).
//   - RegisterApp (ADR-217): on Linux the same desktop entry with no
//     MimeType, marked X-Tela-App=true, kept until UnregisterApp even
//     when its last association goes; on Windows nothing (an app needs no
//     entry there); on macOS LSRegisterURL of the bundle, which is the
//     app's registration (UnregisterApp writes nothing).
//   - Other OSes: [Native] returns platform.UnsupportedAssociations.
//
// # Tests never touch the system
//
// The OS logic takes where it writes as an argument: [LinuxDirs] (the XDG
// directories, and the command runner), a [Registry] (the part of
// HKEY_CURRENT_USER it uses) and [LaunchServices] (the macOS database;
// the Info.plist edited is the given bundle's). Tests point LinuxDirs at
// t.TempDir() and use a [MemRegistry] and a fake Launch Services on
// temporary bundles, which run the Windows and macOS logic on every OS.
// Only Native reads the environment or opens the real registry and Launch
// Services (and the darwin tests, on a temporary bundle of a unique ID).
package assoc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// LinuxDirs are the directories of the XDG Base Directory specification
// that the Linux implementation reads and writes, and how it runs the
// database update commands.
type LinuxDirs struct {
	// DataHome is $XDG_DATA_HOME (default ~/.local/share): applications/
	// and mime/packages/ are written there.
	DataHome string
	// ConfigHome is $XDG_CONFIG_HOME (default ~/.config): mimeapps.list,
	// written only by MakeDefault and Unregister.
	ConfigHome string
	// DataDirs is $XDG_DATA_DIRS (default /usr/local/share, /usr/share),
	// read only: system desktop entries and MIME databases.
	DataDirs []string
	// ConfigDirs is $XDG_CONFIG_DIRS (default /etc/xdg), read only:
	// system mimeapps.list files.
	ConfigDirs []string
	// Desktops are the names of $XDG_CURRENT_DESKTOP, lower-cased, for the
	// <desktop>-mimeapps.list files read first.
	Desktops []string
	// Run runs a command after a change (update-mime-database, the only
	// one). nil runs it with os/exec when it is installed; its failure is
	// ignored either way. Tests record the calls instead.
	Run func(name string, args ...string) error
}

// LinuxDirsFromEnv returns the LinuxDirs of the current session, from the
// XDG environment variables with the specification's defaults.
func LinuxDirsFromEnv() LinuxDirs { return linuxDirs(os.Getenv, os.UserHomeDir) }

// linuxDirs is LinuxDirsFromEnv with its environment as arguments.
func linuxDirs(getenv func(string) string, home func() (string, error)) LinuxDirs {
	h, _ := home()
	dir := func(env, def string) string {
		if v := getenv(env); filepath.IsAbs(v) {
			return v
		}
		return filepath.Join(h, def)
	}
	list := func(env string, def ...string) []string {
		var out []string
		for _, v := range strings.Split(getenv(env), ":") {
			if filepath.IsAbs(v) {
				out = append(out, v)
			}
		}
		if len(out) == 0 {
			return def
		}
		return out
	}
	d := LinuxDirs{
		DataHome:   dir("XDG_DATA_HOME", ".local/share"),
		ConfigHome: dir("XDG_CONFIG_HOME", ".config"),
		DataDirs:   list("XDG_DATA_DIRS", "/usr/local/share", "/usr/share"),
		ConfigDirs: list("XDG_CONFIG_DIRS", "/etc/xdg"),
	}
	for _, n := range strings.Split(getenv("XDG_CURRENT_DESKTOP"), ":") {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			d.Desktops = append(d.Desktops, n)
		}
	}
	return d
}

// Windows returns the associations of Windows written to reg, the current
// user's registry (see the package documentation and windows.go); nil means
// the real HKEY_CURRENT_USER, on Windows only (elsewhere, every method
// returns platform.ErrUnsupported).
func Windows(reg Registry) platform.Associations {
	if reg == nil {
		if nativeRegistry == nil {
			return platform.UnsupportedAssociations{}
		}
		reg = nativeRegistry()
	}
	return &windows{reg: reg}
}

// nativeRegistry returns the real HKEY_CURRENT_USER; set on Windows only
// (registry_windows.go).
var nativeRegistry func() Registry
