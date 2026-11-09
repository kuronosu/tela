//go:build windows

package win32

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// The common item dialogs (IFileOpenDialog, IFileSaveDialog; Vista and
// later) and MessageBoxW. Both run modally, started by runModal outside a
// delivery pass (services.go).

// Vtable indices: IUnknown (0 to 2), IModalWindow::Show (3), IFileDialog
// (4 to 26), IFileOpenDialog (27 on); IShellItem and IShellItemArray after
// IUnknown (shobjidl_core.h).
const (
	vtFileDialogShow                = 3
	vtFileDialogSetFileTypes        = 4
	vtFileDialogSetFileTypeIndex    = 5
	vtFileDialogSetOptions          = 9
	vtFileDialogGetOptions          = 10
	vtFileDialogSetFolder           = 12
	vtFileDialogSetFileName         = 15
	vtFileDialogSetTitle            = 17
	vtFileDialogGetResult           = 20
	vtFileDialogSetDefaultExtension = 22
	vtFileDialogClose               = 23
	vtFileOpenDialogGetResults      = 27

	vtShellItemGetDisplayName = 5
	vtShellItemArrayGetCount  = 7
	vtShellItemArrayGetItemAt = 8

	sigdnFileSysPath = 0x80058000
)

// comdlgFilterSpec is COMDLG_FILTERSPEC.
type comdlgFilterSpec struct {
	name, spec *uint16
}

// showFileDialog shows a common item dialog and returns the chosen paths:
// nil with a nil error when the user canceled.
func (b *Backend) showFileDialog(kind fileDialogKind, opts platform.FileDialogOptions) ([]string, error) {
	clsid, iid, what := &clsidFileOpenDialog, &iidFileOpenDialog, "FileOpenDialog"
	if kind == dialogSaveFile {
		clsid, iid, what = &clsidFileSaveDialog, &iidFileSaveDialog, "FileSaveDialog"
	}
	var dlg comObject
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&dlg)))
	if hr := int32(uint32(r)); hr < 0 {
		return nil, hresultError("creating the "+what, hr)
	}
	defer dlg.release()

	var fos uint32
	if hr := dlg.call(vtFileDialogGetOptions, uintptr(unsafe.Pointer(&fos))); hr < 0 {
		return nil, hresultError("IFileDialog.GetOptions", hr)
	}
	if hr := dlg.call(vtFileDialogSetOptions, uintptr(fileDialogFlags(kind, fos, opts.Multiple))); hr < 0 {
		return nil, hresultError("IFileDialog.SetOptions", hr)
	}
	// The rest only presets the dialog: a failure leaves the dialog's own
	// choice.
	if opts.Title != "" {
		dlg.call(vtFileDialogSetTitle, uintptr(unsafe.Pointer(utf16(opts.Title))))
	}
	if kind != dialogPickFolder && len(opts.Filters) > 0 {
		specs := filterSpecs(opts.Filters)
		native := make([]comdlgFilterSpec, len(specs))
		for i, s := range specs {
			native[i] = comdlgFilterSpec{name: utf16(s.name), spec: utf16(s.spec)}
		}
		dlg.call(vtFileDialogSetFileTypes, uintptr(len(native)), uintptr(unsafe.Pointer(&native[0])))
		dlg.call(vtFileDialogSetFileTypeIndex, 1) // 1-based
		if ext := defaultExtension(opts.Filters); kind == dialogSaveFile && ext != "" {
			dlg.call(vtFileDialogSetDefaultExtension, uintptr(unsafe.Pointer(utf16(ext))))
		}
	}
	if opts.Directory != "" {
		if folder := shellItemFromPath(opts.Directory); folder != 0 {
			dlg.call(vtFileDialogSetFolder, uintptr(folder))
			folder.release()
		}
	}
	if kind == dialogSaveFile && opts.Name != "" {
		dlg.call(vtFileDialogSetFileName, uintptr(unsafe.Pointer(utf16(opts.Name))))
	}

	b.desk.dialog = dlg
	hr := dlg.call(vtFileDialogShow, ownerOf(opts.Parent))
	b.desk.dialog = 0
	switch {
	case hr == hrCanceled:
		return nil, nil
	case hr < 0:
		return nil, hresultError("IFileDialog.Show", hr)
	}

	if kind == dialogOpenFiles {
		return openDialogResults(dlg)
	}
	var item comObject
	if hr := dlg.call(vtFileDialogGetResult, uintptr(unsafe.Pointer(&item))); hr < 0 {
		return nil, hresultError("IFileDialog.GetResult", hr)
	}
	defer item.release()
	path, err := shellItemPath(item)
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

// openDialogResults returns the paths chosen in an open dialog.
func openDialogResults(dlg comObject) ([]string, error) {
	var items comObject
	if hr := dlg.call(vtFileOpenDialogGetResults, uintptr(unsafe.Pointer(&items))); hr < 0 {
		return nil, hresultError("IFileOpenDialog.GetResults", hr)
	}
	defer items.release()
	var n uint32
	if hr := items.call(vtShellItemArrayGetCount, uintptr(unsafe.Pointer(&n))); hr < 0 {
		return nil, hresultError("IShellItemArray.GetCount", hr)
	}
	paths := make([]string, 0, n)
	for i := range n {
		var item comObject
		if hr := items.call(vtShellItemArrayGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item))); hr < 0 {
			return nil, hresultError("IShellItemArray.GetItemAt", hr)
		}
		path, err := shellItemPath(item)
		item.release()
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	return paths, nil
}

// shellItemPath returns the file system path of a shell item. The dialogs
// have FOS_FORCEFILESYSTEM, so every result has one.
func shellItemPath(item comObject) (string, error) {
	var p uintptr
	if hr := item.call(vtShellItemGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))); hr < 0 {
		return "", hresultError("IShellItem.GetDisplayName", hr)
	}
	return coTaskString(p), nil
}

// shellItemFromPath returns the shell item of a folder, or 0 when the path
// does not name one (the dialog then starts where it would have).
func shellItemFromPath(path string) comObject {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0
	}
	var item comObject
	r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(utf16(abs))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if int32(uint32(r)) < 0 {
		return 0
	}
	return item
}

// showMessageBox shows a MessageBoxW. Its modal loop ends on WM_QUIT,
// which it posts again, so Quit closes it. The buttons the app translated
// (messageBoxLabels) are renamed as the box is created.
func (b *Backend) showMessageBox(opts platform.MessageBoxOptions) (platform.MessageAnswer, error) {
	owner := ownerOf(opts.Parent)
	if labels := messageBoxLabels(opts.Buttons, b.desk.strings); len(labels) > 0 {
		defer installMessageBoxHook(labels)()
	}
	r, _, e := procMessageBoxW.Call(owner, uintptr(unsafe.Pointer(utf16(opts.Text))), uintptr(unsafe.Pointer(utf16(opts.Title))),
		uintptr(messageBoxStyle(opts, owner != 0)))
	answer, ok := messageBoxAnswer(int32(r), opts.Buttons)
	if !ok {
		return answer, winError("MessageBoxW", e)
	}
	return answer, nil
}

// The hook that renames the buttons of a message box (ADR-251). MessageBoxW
// has no way to give its buttons other text, and the OS's own language is
// the user's, not the app's; a WH_CBT hook on the UI thread sees the box
// being activated and sets the texts of its buttons (SetDlgItemTextW) before
// it is shown. Message boxes run one at a time on the UI thread (runModal),
// so the labels are package state.
const (
	whCBT        = 5
	hcbtActivate = 5
	// messageBoxClass is the window class of a message box (a dialog).
	messageBoxClass = "#32770"
)

var messageBoxHook struct {
	labels []messageBoxLabel
	hook   uintptr
	proc   uintptr // the callback, made once (callbacks are never freed)
}

// installMessageBoxHook hooks the UI thread for the next message box and
// returns the function that removes the hook. If the hook cannot be set,
// the box keeps the labels of the user's Windows.
func installMessageBoxHook(labels []messageBoxLabel) (remove func()) {
	h := &messageBoxHook
	if h.proc == 0 {
		h.proc = syscall.NewCallback(messageBoxHookProc)
	}
	h.labels = labels
	hook, _, _ := procSetWindowsHookExW.Call(whCBT, h.proc, 0, uintptr(windows.GetCurrentThreadId()))
	h.hook = hook
	return func() {
		if h.hook != 0 {
			procUnhookWindowsHookEx.Call(h.hook)
		}
		h.hook, h.labels = 0, nil
	}
}

// messageBoxHookProc is the WH_CBT callback: on the activation of the
// message box it renames the buttons, once.
func messageBoxHookProc(code, wParam, lParam uintptr) uintptr {
	h := &messageBoxHook
	if int32(code) == hcbtActivate && len(h.labels) > 0 {
		var class [16]uint16
		n, _, _ := procDialogClassNameW.Call(wParam, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if syscall.UTF16ToString(class[:n]) == messageBoxClass {
			for _, l := range h.labels {
				procSetDlgItemTextW.Call(wParam, uintptr(l.id), uintptr(unsafe.Pointer(utf16(l.text))))
			}
			h.labels = nil
		}
	}
	r, _, _ := procCallNextHookEx.Call(h.hook, code, wParam, lParam)
	return r
}
