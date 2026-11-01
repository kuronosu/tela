package win32

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestTrashOutcome(t *testing.T) {
	const path = `C:\Users\ana\todo.md`
	if err := trashOutcome(path, sOK, false, false, sOK); err != nil {
		t.Errorf("a recycled item = %v", err)
	}
	// The sink refused it: the shell would have deleted it for good.
	err := trashOutcome(path, hrCopyEngineUserCancelled, true, true, hrAbort)
	if !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("a refused item = %v, want ErrUnsupported", err)
	}
	notFound := int32(-0x7FF8FFFE) // HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND), 0x80070002
	if err := trashOutcome(path, sOK, false, false, notFound); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing item = %v, want fs.ErrNotExist", err)
	}
	denied := int32(-0x7FF8FFFB) // 0x80070005
	if err := trashOutcome(path, denied, true, false, 0); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("a denied item = %v, want fs.ErrPermission", err)
	}
	if err := trashOutcome(path, sOK, true, false, 0); err == nil || errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("an aborted operation = %v, want an error", err)
	}
	if err := trashHRError(path, hrAbort); err == nil || err.Error() != `win32: MoveToTrash("C:\\Users\\ana\\todo.md"): the operation was canceled` {
		t.Errorf("trashHRError(E_ABORT) = %v", err)
	}
	if hr := hrAbort; uint32(eAbort) != uint32(hr) {
		t.Errorf("eAbort %#x and hrAbort %#x differ", uint32(eAbort), uint32(hr))
	}
	if trashFlags != 0x20080654 {
		t.Errorf("trashFlags = %#x", trashFlags)
	}
}
