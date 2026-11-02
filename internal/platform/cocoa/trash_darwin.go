//go:build darwin

package cocoa

import (
	"runtime"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The trash is -[NSFileManager trashItemAtURL:resultingItemURL:error:]
// (ADR-216, ADR-224), what the Finder's Move to Trash does: the item is
// moved into the trash of its volume (~/.Trash for the startup disk),
// renamed when the trash holds the name already, and the Finder's Put
// Back can restore it. It never deletes: on a volume without a trash (a
// network share, a read-only disk) it fails and leaves the item in place,
// which is ErrUnsupported (services.go maps the errors). The URL is made
// with fileURLWithPath:isDirectory:, which does not resolve symbolic
// links, and the file manager moves the link itself, not its target (the
// darwin test checks both). NSFileManager's default manager is safe from
// any thread, so MoveToTrash is safe from any goroutine; it blocks for
// the move (a rename on the same volume).

var trashSel = struct{ defaultManager, trashItem darwin.SEL }{
	defaultManager: darwin.Sel("defaultManager"),
	trashItem:      darwin.Sel("trashItemAtURL:resultingItemURL:error:"),
}

// MoveToTrash moves the file or folder at path to its volume's trash.
func (desktop) MoveToTrash(path string) error {
	_, err := moveToTrash(path)
	return err
}

// moveToTrash moves path to the trash and returns where it went.
func moveToTrash(path string) (string, error) {
	clean, fi, err := platform.CheckTrash(path)
	if err != nil {
		return "", err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	manager := darwin.SendClass(darwin.GetClass("NSFileManager"), trashSel.defaultManager)
	// A symbolic link is not a folder, whatever it points to.
	url := darwin.FileURL(clean, fi.IsDir())
	// The autoreleased out-parameters, on the heap (which does not move).
	out := new([2]darwin.ID)
	if !darwin.SendBool(manager, trashSel.trashItem, url, unsafe.Pointer(&out[0]), unsafe.Pointer(&out[1])) {
		domain, code, desc := darwin.ErrorInfo(out[1])
		return "", trashError(clean, domain, code, desc)
	}
	return darwin.URLPath(out[0]), nil
}
