package win32

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of MoveToTrash (ADR-216); trash_windows.go makes the
// calls.
//
// Why IFileOperation and not SHFileOperationW: both move a file to the
// Recycle Bin with FOF_ALLOWUNDO, but when the Recycle Bin cannot take
// it (a network drive, a removable one without a bin, a file larger than
// the bin's quota) both delete it for good instead. SHFileOperationW does
// so silently with FOF_NOCONFIRMATION, or shows the "permanently delete?"
// question with FOF_WANTNUKEWARNING; it has no way to just refuse.
// IFileOperation reports every item to an IFileOperationProgressSink
// first: PreDeleteItem gets TSF_DELETE_RECYCLE_IF_POSSIBLE only for an
// item that will be recycled, and returning E_ABORT for any other stops
// the operation before anything is deleted (Chromium's and Electron's
// trash do the same). The contract says MoveToTrash never deletes, so the
// price (COM on a thread of its own, a Go COM object) is worth it.

// IFileOperation flags (shobjidl_core.h, shellapi.h).
const (
	fofSilent           = 0x0004
	fofNoConfirmation   = 0x0010
	fofAllowUndo        = 0x0040
	fofNoConfirmMkdir   = 0x0200
	fofNoErrorUI        = 0x0400
	fofNoUI             = fofSilent | fofNoConfirmation | fofNoConfirmMkdir | fofNoErrorUI
	fofxRecycleOnDelete = 0x00080000
	fofxAddUndoRecord   = 0x20000000

	// trashFlags: recycle, never ask, record the undo (Ctrl+Z in
	// Explorer brings the file back).
	trashFlags = fofAllowUndo | fofNoUI | fofxRecycleOnDelete | fofxAddUndoRecord
)

// HRESULTs of the trash.
const (
	eAbort                     = 0x80004004         // E_ABORT, as a COM server returns it
	hrAbort                    = int32(-0x7FFFBFFC) // E_ABORT, 0x80004004
	hrCopyEngineUserCancelled  = int32(-0x7FD90000) // COPYENGINE_E_USER_CANCELLED, 0x80270000
	hrClassNotRegistered       = int32(-0x7FFBFEAC) // REGDB_E_CLASSNOTREG, 0x80040154
	win32ErrorFileNotFound     = 2
	win32ErrorPathNotFound     = 3
	win32ErrorAccessDenied     = 5
	win32ErrorSharingViolation = 32
	win32ErrorLockViolation    = 33
)

// errWouldDelete is the reason a file stays in place: the shell would
// have deleted it instead of recycling it.
var errWouldDelete = errors.New("the Recycle Bin cannot take it (a network drive, a drive without a Recycle Bin, or a file larger than the bin), and Windows would delete it for good instead")

// trashOutcome turns what the file operation reported into MoveToTrash's
// result. perform is PerformOperations' HRESULT, aborted its
// GetAnyOperationsAborted, refused whether the progress sink stopped the
// item (it would not be recycled), and item the HRESULT PostDeleteItem
// got (0 when it was not called).
func trashOutcome(path string, perform int32, aborted, refused bool, item int32) error {
	switch {
	case refused:
		return fmt.Errorf("win32: MoveToTrash(%q): %v; it was left in place: %w", path, errWouldDelete, platform.ErrUnsupported)
	case item < 0:
		return trashHRError(path, item)
	case perform < 0:
		return trashHRError(path, perform)
	case aborted:
		return fmt.Errorf("win32: MoveToTrash(%q): the shell stopped the operation; nothing was moved", path)
	}
	return nil
}

// trashHRError describes a failed HRESULT of the trash: missing files wrap
// fs.ErrNotExist and denied access fs.ErrPermission, like the rest of the
// backend.
func trashHRError(path string, hr int32) error {
	var reason error
	code, win := hresultFacilityWin32(hr)
	switch {
	case win && (code == win32ErrorFileNotFound || code == win32ErrorPathNotFound):
		reason = fs.ErrNotExist
	case win && code == win32ErrorAccessDenied:
		reason = fs.ErrPermission
	case win && (code == win32ErrorSharingViolation || code == win32ErrorLockViolation):
		reason = errors.New("it is in use by another program")
	case hr == hrAbort || hr == hrCopyEngineUserCancelled:
		reason = errors.New("the operation was canceled")
	default:
		reason = errors.New(hresultString(hr))
	}
	return fmt.Errorf("win32: MoveToTrash(%q): %w", path, reason)
}
