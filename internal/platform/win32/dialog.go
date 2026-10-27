package win32

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the file dialogs and message boxes (dialog_windows.go
// shows them): option flags, the file type list, and the mapping of
// message box styles and answers.

// FILEOPENDIALOGOPTIONS flags (shobjidl.h).
const (
	fosOverwritePrompt  = 0x00000002
	fosNoChangeDir      = 0x00000008
	fosPickFolders      = 0x00000020
	fosForceFileSystem  = 0x00000040
	fosAllowMultiSelect = 0x00000200
	fosPathMustExist    = 0x00000800
	fosFileMustExist    = 0x00001000
)

// fileDialogKind is the kind of a common item dialog.
type fileDialogKind uint8

const (
	dialogOpenFiles fileDialogKind = iota
	dialogSaveFile
	dialogPickFolder
)

// fileDialogFlags returns the options of a dialog of the given kind: the
// dialog's defaults (base, from GetOptions) plus file system items only
// (FOS_FORCEFILESYSTEM: every result has a path), the process's current
// directory left alone (FOS_NOCHANGEDIR) and what the kind needs.
func fileDialogFlags(kind fileDialogKind, base uint32, multiple bool) uint32 {
	f := base | fosForceFileSystem | fosNoChangeDir | fosPathMustExist
	switch kind {
	case dialogOpenFiles:
		f |= fosFileMustExist
		if multiple {
			f |= fosAllowMultiSelect
		}
	case dialogSaveFile:
		f |= fosOverwritePrompt
	case dialogPickFolder:
		f |= fosPickFolders
	}
	return f
}

// filterSpec is one COMDLG_FILTERSPEC: the label of a file type and its
// patterns separated by ';'.
type filterSpec struct{ name, spec string }

// filterSpecs converts the contract's filters (already checked: no pattern
// is empty or holds ';' or NUL) to the dialog's file types. A filter
// without a name is labeled with its patterns.
func filterSpecs(filters []platform.FileFilter) []filterSpec {
	specs := make([]filterSpec, len(filters))
	for i, f := range filters {
		spec := strings.Join(f.Patterns, ";")
		name := f.Name
		if name == "" {
			name = spec
		}
		specs[i] = filterSpec{name: sanitizeTitle(name), spec: sanitizeTitle(spec)}
	}
	return specs
}

// defaultExtension returns the extension a save dialog appends to a name
// typed without one: that of the first filter's first pattern when it is
// a plain "*.ext" ("" otherwise, and then nothing is appended). Once the
// dialog has a default extension, it uses the extension of the file type
// the user selects instead.
func defaultExtension(filters []platform.FileFilter) string {
	if len(filters) == 0 || len(filters[0].Patterns) == 0 {
		return ""
	}
	ext, ok := strings.CutPrefix(filters[0].Patterns[0], "*.")
	if !ok || ext == "" || strings.ContainsAny(ext, "*?.") {
		return ""
	}
	return ext
}

// HRESULTs of the dialogs.
const (
	// hrCanceled is HRESULT_FROM_WIN32(ERROR_CANCELLED): the user closed
	// the dialog without choosing.
	hrCanceled = int32(-0x7FF8FB39) // 0x800704C7
	// hrChangedMode is RPC_E_CHANGED_MODE: the thread's COM apartment is
	// multithreaded.
	hrChangedMode = int32(-0x7FFEFEFA) // 0x80010106
	sOK           = 0
	sFalse        = 1
)

// hresultFacilityWin32 reports whether hr wraps a Win32 error code
// (HRESULT_FROM_WIN32), and returns the code.
func hresultFacilityWin32(hr int32) (code uint32, ok bool) {
	if uint32(hr)&0xFFFF0000 == 0x80070000 {
		return uint32(hr) & 0xFFFF, true
	}
	return 0, false
}

// hresultString formats an HRESULT the way Windows documents them.
func hresultString(hr int32) string { return fmt.Sprintf("HRESULT 0x%08X", uint32(hr)) }

// MessageBoxW styles and answers (winuser.h).
const (
	mbOK              = 0x00000000
	mbOKCancel        = 0x00000001
	mbYesNoCancel     = 0x00000003
	mbYesNo           = 0x00000004
	mbIconError       = 0x00000010
	mbIconQuestion    = 0x00000020
	mbIconWarning     = 0x00000030
	mbIconInformation = 0x00000040
	mbSetForeground   = 0x00010000

	idOK     = 1
	idCancel = 2
	idYes    = 6
	idNo     = 7
)

// messageBoxStyle returns the MessageBoxW style of a box: its buttons and
// icon. A box without an owner window is brought to the foreground
// (MB_SETFOREGROUND), since nothing else would raise it (a tray-only app);
// one with an owner is modal to it (MB_APPLMODAL, the zero style).
func messageBoxStyle(opts platform.MessageBoxOptions, owned bool) uint32 {
	var s uint32
	switch opts.Buttons {
	case platform.ButtonsOK:
		s = mbOK
	case platform.ButtonsOKCancel:
		s = mbOKCancel
	case platform.ButtonsYesNo:
		s = mbYesNo
	case platform.ButtonsYesNoCancel:
		s = mbYesNoCancel
	}
	switch opts.Kind {
	case platform.MessageInfo:
		s |= mbIconInformation
	case platform.MessageWarning:
		s |= mbIconWarning
	case platform.MessageError:
		s |= mbIconError
	case platform.MessageQuestion:
		s |= mbIconQuestion
	}
	if !owned {
		s |= mbSetForeground
	}
	return s
}

// messageBoxAnswer maps the result of MessageBoxW to the contract's answer.
// 0 is a failure (ok false). Escape and the close button give IDCANCEL in
// sets with Cancel and IDOK for a lone OK (Windows' rule, which is also
// the contract's); a result that is not a button of the set, which Windows
// does not give, is the set's dismiss answer.
func messageBoxAnswer(id int32, buttons platform.MessageButtons) (answer platform.MessageAnswer, ok bool) {
	switch id {
	case 0:
		return buttons.DismissAnswer(), false
	case idOK:
		answer = platform.AnswerOK
	case idCancel:
		answer = platform.AnswerCancel
	case idYes:
		answer = platform.AnswerYes
	case idNo:
		answer = platform.AnswerNo
	default:
		return buttons.DismissAnswer(), true
	}
	if !buttons.Has(answer) {
		return buttons.DismissAnswer(), true
	}
	return answer, true
}
