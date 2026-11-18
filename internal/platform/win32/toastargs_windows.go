//go:build windows

package win32

import "os"

// startedByToast says that COM started this process for a toast clicked
// after the app exited: its command line had the activator's flag, which
// init removed.
var startedByToast bool

// init removes the toast activator's flags from os.Args before the app's
// main runs (every Tela program on Windows imports this package), so an
// app that parses its flags with package flag starts as at any start
// instead of failing on "-ToastActivated" or "-Embedding". The click
// itself arrives as a NotificationEvent marked Earlier.
func init() {
	os.Args, startedByToast = stripToastFlags(os.Args)
}
