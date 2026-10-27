//go:build windows

package win32

import (
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// shellOpen opens target (a URI or an absolute path) with ShellExecuteW
// and the "open" verb: the default browser or mail client for a URI, the
// application associated with a file, Explorer for a folder. It returns
// when the shell has started the application (or handed the request to a
// running one). ShellExecuteW reports failures as codes, which
// shellExecuteError maps.
//
// COM is joined first, as the ShellExecuteW documentation asks: shell
// extensions that a target may need are COM objects. A thread in the
// multithreaded apartment still works for the usual targets.
func (b *Backend) shellOpen(parent platform.Window, target string) error {
	_ = b.desk.initCOM()
	const swShowNormal = 1
	r, _, _ := procShellExecuteW.Call(ownerOf(parent), uintptr(unsafe.Pointer(utf16("open"))),
		uintptr(unsafe.Pointer(utf16(target))), 0, 0, swShowNormal)
	return shellExecuteError(r, target)
}
