//go:build windows

package win32

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of drag and drop, clipboard data and menus (phase 13). They
// open real windows, use the real clipboard and, for the drag and the
// popup menus, move the real mouse and type on the real keyboard with
// SendInput, so they need an attended Windows desktop: they run with
// TELA_LIVE=1 only, and not under Wine.

var (
	procSetTimer         = user32.NewProc("SetTimer")
	procKillTimer        = user32.NewProc("KillTimer")
	procGetMenuItemCount = user32.NewProc("GetMenuItemCount")
	procGetMenuStringW   = user32.NewProc("GetMenuStringW")
	procGetSubMenu       = user32.NewProc("GetSubMenu")
	procGetMenuState     = user32.NewProc("GetMenuState")
)

const (
	mfByPosition = 0x0400
	scArrowDown  = 0xE050
)

func requireLiveIntegration(t *testing.T) {
	t.Helper()
	requireInteractiveDesktop(t)
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real clipboard, OLE and menus (they need an attended desktop)")
	}
	if inWine() {
		t.Skip("Wine's OLE and menus are not the ones these tests check; they run on Windows")
	}
}

// onUIThreadSoon runs fn once from Run's message loop, outside any
// delivery pass (a thread timer's TIMERPROC, dispatched by
// DispatchMessageW), as OLE calls a drop target.
func onUIThreadSoon(fn func()) {
	var id uintptr
	cb := syscall.NewCallback(func(hwnd, msg, timer, tick uintptr) uintptr {
		procKillTimer.Call(0, id)
		fn()
		return 0
	})
	id, _, _ = procSetTimer.Call(0, 0, 10, cb)
}

// callDropTarget calls method m of an IDropTarget with a POINTL argument
// in the middle, as OLE does.
func callDropTarget(target comObject, m int, before []uintptr, pt point, effect *uint32) int32 {
	args := append(append(slices.Clone(before), pointlArgs(pt)...), uintptr(unsafe.Pointer(effect)))
	return target.call(m, args...)
}

func TestLiveDropTargetCalls(t *testing.T) {
	requireLiveIntegration(t)
	b := liveBackend(t)
	var w *Window
	var got []platform.DragEvent
	started := false
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "tela drop target", Size: platform.Size{W: 240, H: 160}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(pw platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, pw, solidBlack)
			if !started {
				started = true
				onUIThreadSoon(func() { dropTargetScript(t, b, w); b.Quit() })
			}
		case platform.DragEvent:
			got = append(got, e)
			switch e.Action {
			case platform.DragEnter, platform.DragOver:
				if e.Mods&platform.ModShift != 0 {
					pw.SetDropEffect(platform.DropMove, "application/x-b")
				} else {
					pw.SetDropEffect(platform.DropCopy, "application/x-b")
				}
			}
		}
	})
	want := []string{"enter false", "over false", "drop false [application/x-b]", "enter true", "leave true"}
	var have []string
	for _, e := range got {
		s := fmt.Sprintf("%v %v", e.Action, e.FromApp)
		if e.Action == platform.DragDrop {
			s += fmt.Sprintf(" %q", e.Data.Types())
			s = strings.ReplaceAll(s, `"`, "")
		}
		have = append(have, s)
	}
	if !slices.Equal(have, want) {
		t.Errorf("drag events = %q, want %q", have, want)
	}
}

// dropTargetScript plays OLE against the window's drop target: a foreign
// drag that drops, then a drag of this app that leaves.
func dropTargetScript(t *testing.T, b *Backend, w *Window) {
	if w.integ.target == nil {
		t.Errorf("the window has no registered drop target (OleInitialize or RegisterDragDrop failed)")
		return
	}
	target := comObject(w.integ.target.ptr())
	r := getWindowRect(w.hwnd)
	pt := point{r.left + r.width()/2, r.top + r.height()/2}

	foreign := newForeignDataObject(platform.Data{
		{Type: platform.MIMEText, Bytes: []byte("text")},
		{Type: "application/x-b", Bytes: []byte("B")},
	})
	defer foreign.release()
	effect := uint32(platform.DropCopy | platform.DropMove)
	callDropTarget(target, 3, []uintptr{foreign.ptr(), mkLButton}, pt, &effect)
	if effect != uint32(platform.DropCopy) {
		t.Errorf("DragEnter answered %#x, want DROPEFFECT_COPY (the handler's answer)", effect)
	}
	// OLE's timer: nothing changed, no event.
	effect = uint32(platform.DropCopy | platform.DropMove)
	callDropTarget(target, 4, []uintptr{mkLButton}, pt, &effect)
	if effect != uint32(platform.DropCopy) {
		t.Errorf("unchanged DragOver answered %#x", effect)
	}
	// Shift: move.
	effect = uint32(platform.DropCopy | platform.DropMove)
	callDropTarget(target, 4, []uintptr{mkLButton | mkShift}, point{pt.x + 3, pt.y}, &effect)
	if effect != uint32(platform.DropMove) {
		t.Errorf("DragOver with Shift answered %#x, want DROPEFFECT_MOVE", effect)
	}
	effect = uint32(platform.DropCopy | platform.DropMove)
	callDropTarget(target, 6, []uintptr{foreign.ptr(), mkShift}, point{pt.x + 3, pt.y}, &effect)
	if effect != uint32(platform.DropMove) {
		t.Errorf("Drop returned %#x, want DROPEFFECT_MOVE", effect)
	}

	own := newDataObject(platform.TextData("own"))
	defer own.release()
	effect = uint32(platform.DropCopy)
	callDropTarget(target, 3, []uintptr{own.ptr(), mkLButton}, pt, &effect)
	if hr := target.call(5); hr != sOK {
		t.Errorf("DragLeave = %#x", uint32(hr))
	}
}

// clientCenter returns the center of a window's content, in screen pixels.
func clientCenter(w *Window) image.Point {
	o := clientOrigin(w.hwnd)
	c := getClientRect(w.hwnd)
	return image.Pt(o.X+int(c.width())/2, o.Y+int(c.height())/2)
}

// mouseInput and mouseInputRec are MOUSEINPUT and INPUT (the size of its
// largest member, MOUSEINPUT).
type mouseInput struct {
	dx, dy          int32
	data, flags, tm uint32
	extra           uintptr
}

type mouseInputRec struct {
	typ uint32
	mi  mouseInput
}

const (
	inputMouse            = 0
	mouseeventfMove       = 0x0001
	mouseeventfLeftDown   = 0x0002
	mouseeventfLeftUp     = 0x0004
	mouseeventfVirtualDsk = 0x4000
	mouseeventfAbsolute   = 0x8000
)

// sendMouse moves the real mouse to p (screen pixels) and adds the button
// flags.
func sendMouse(p image.Point, buttons uint32) {
	vx, _, _ := procGetSystemMetrics.Call(76) // SM_XVIRTUALSCREEN
	vy, _, _ := procGetSystemMetrics.Call(77)
	vw, _, _ := procGetSystemMetrics.Call(78)
	vh, _, _ := procGetSystemMetrics.Call(79)
	x := (int64(p.X) - int64(int32(vx))) * 65535 / max(int64(int32(vw))-1, 1)
	y := (int64(p.Y) - int64(int32(vy))) * 65535 / max(int64(int32(vh))-1, 1)
	in := mouseInputRec{typ: inputMouse, mi: mouseInput{dx: int32(x), dy: int32(y), flags: mouseeventfMove | mouseeventfAbsolute | mouseeventfVirtualDsk | buttons}}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func TestLiveDragBetweenWindows(t *testing.T) {
	requireLiveIntegration(t)
	var old point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&old)))
	t.Cleanup(func() { procSetCursorPos.Call(uintptr(int(old.x)), uintptr(int(old.y))) })

	b := liveBackend(t)
	var src, dst *Window
	var started atomic.Bool
	var log []string
	var dropped platform.Data
	done := platform.DropEffect(0xFF)
	framed := map[*Window]bool{}
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	name := func(w platform.Window) string {
		if w == platform.Window(src) {
			return "src"
		}
		return "dst"
	}
	runLive(t, b, func() {
		a, err := b.NewWindow(platform.WindowOptions{Title: "tela drag source", Size: platform.Size{W: 200, H: 150}, HasPosition: true, Position: image.Pt(120, 160)})
		if err != nil {
			t.Fatal(err)
		}
		c, err := b.NewWindow(platform.WindowOptions{Title: "tela drag target", Size: platform.Size{W: 200, H: 150}, HasPosition: true, Position: image.Pt(460, 160)})
		if err != nil {
			t.Fatal(err)
		}
		src, dst = a.(*Window), c.(*Window)
	}, func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, solidBlack)
			framed[w.(*Window)] = true
			if len(framed) == 2 && !framed[nil] {
				framed[nil] = true
				from, to := clientCenter(src), clientCenter(dst)
				go func() {
					time.Sleep(300 * time.Millisecond)
					sendMouse(from, 0)
					time.Sleep(100 * time.Millisecond)
					sendMouse(from, mouseeventfLeftDown)
					for i := 0; i < 100 && !started.Load(); i++ {
						time.Sleep(20 * time.Millisecond)
					}
					for i := 1; i <= 12; i++ {
						sendMouse(from.Add(to.Sub(from).Mul(i).Div(12)), 0)
						time.Sleep(40 * time.Millisecond)
					}
					time.Sleep(300 * time.Millisecond)
					sendMouse(to, mouseeventfLeftUp)
				}()
			}
		case platform.PointerEvent:
			if w == platform.Window(src) && (e.Action == platform.PointerDown || e.Action == platform.PointerCancel) {
				log = append(log, fmt.Sprintf("src pointer %v", e.Action))
			}
			if w == platform.Window(src) && e.Action == platform.PointerDown && !started.Load() {
				err := src.StartDrag(platform.DragOptions{
					Data:    platform.Data{{Type: "application/x-tela-test", Bytes: []byte("item 7")}, {Type: platform.MIMEText, Bytes: []byte("item 7")}},
					Allowed: platform.DropCopy | platform.DropMove,
					Image:   img, Hotspot: image.Pt(8, 8),
				})
				if err != nil {
					t.Errorf("StartDrag: %v", err)
					b.Quit()
					return
				}
				started.Store(true)
				if err := src.StartDrag(platform.DragOptions{Data: platform.TextData("x"), Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
					t.Errorf("a second StartDrag = %v, want ErrNoGrab", err)
				}
			}
		case platform.DragEvent:
			if e.Action != platform.DragOver {
				log = append(log, fmt.Sprintf("%s %v fromApp=%v", name(w), e.Action, e.FromApp))
			}
			if w == platform.Window(dst) {
				switch e.Action {
				case platform.DragEnter, platform.DragOver:
					w.SetDropEffect(platform.DropMove, "application/x-tela-test")
				case platform.DragDrop:
					dropped = e.Data
				}
			}
		case platform.DragDoneEvent:
			log = append(log, fmt.Sprintf("%s done %v", name(w), e.Effect))
			done = e.Effect
			b.Quit()
		}
	})
	want := []string{
		"src pointer down", "src pointer cancel",
		"src enter fromApp=true", "src leave fromApp=true",
		"dst enter fromApp=true", "dst drop fromApp=true",
		"src done move",
	}
	if !slices.Equal(log, want) {
		t.Errorf("events:\n  %s\nwant:\n  %s", strings.Join(log, "\n  "), strings.Join(want, "\n  "))
	}
	if done != platform.DropMove {
		t.Errorf("DragDoneEvent = %v, want move", done)
	}
	if len(dropped) != 1 || dropped[0].Type != "application/x-tela-test" || string(dropped[0].Bytes) != "item 7" {
		t.Errorf("dropped data = %q", dropped)
	}
}

func TestLiveClipboardDataFormats(t *testing.T) {
	requireLiveIntegration(t)
	img := testImage()
	var pngBytes bytes.Buffer
	png.Encode(&pngBytes, img)
	dir := t.TempDir()
	files := []string{filepath.Join(dir, "a b.txt"), filepath.Join(dir, "ü.png")}
	files0, err := platform.FileData(files...)
	if err != nil {
		t.Fatal(err)
	}
	uriList, _ := files0.Get(platform.MIMEURIList)
	data := platform.Data{
		{Type: platform.MIMEHTML, Bytes: []byte("<b>tela ✓</b>")},
		{Type: platform.MIMEText, Bytes: []byte("tela ✓\r\nline")},
		{Type: platform.MIMEPNG, Bytes: pngBytes.Bytes()},
		{Type: platform.MIMEURIList, Bytes: uriList},
		{Type: "application/x-tela-test", Bytes: []byte{0, 1, 2, 0xFF}},
	}
	var types []string
	read := map[string][]byte{}
	var missing []byte = []byte("unset")
	var afterClear []string = []string{"unset"}
	clipboardSession(t, func(b *Backend, quit func()) {
		cb := b.Clipboard()
		if err := cb.Write(data); err != nil {
			t.Errorf("Write: %v", err)
			quit()
			return
		}
		// What other programs see.
		for _, f := range []uint32{cfUnicodeText, cfHDrop, cfDIBV5, cfDIB, registerFormat(cfNameHTML), registerFormat(cfNamePNG)} {
			if r, _, _ := procIsClipboardFormatAvailable.Call(uintptr(f)); r == 0 {
				t.Errorf("format %d (%s) is not on the clipboard", f, clipFormatOf(f).describe())
			}
		}
		cb.ReadTypes(func(got []string, err error) {
			if err != nil {
				t.Errorf("ReadTypes: %v", err)
			}
			types = got
		})
		for _, e := range data {
			cb.Read(e.Type, func(got []byte, err error) {
				if err != nil {
					t.Errorf("Read(%s): %v", e.Type, err)
				}
				read[e.Type] = got
			})
		}
		cb.Read("application/x-absent", func(got []byte, err error) {
			if err != nil {
				t.Errorf("Read(absent): %v", err)
			}
			missing = got
			if err := cb.Write(nil); err != nil {
				t.Errorf("Write(nil): %v", err)
			}
			cb.ReadTypes(func(got []string, err error) {
				afterClear = got
				quit()
			})
		})
	})
	if !slices.Equal(types, data.Types()) {
		t.Errorf("ReadTypes = %q, want %q", types, data.Types())
	}
	for _, e := range data {
		got := read[e.Type]
		switch e.Type {
		case platform.MIMEPNG:
			checkPNGPixels(t, got, img) // the PNG as written
		case "application/x-tela-test":
			if !bytes.HasPrefix(got, e.Bytes) {
				t.Errorf("Read(%s) = %v, want %v", e.Type, got, e.Bytes)
			}
		default:
			if !bytes.Equal(got, e.Bytes) {
				t.Errorf("Read(%s) = %q, want %q", e.Type, got, e.Bytes)
			}
		}
	}
	if missing != nil {
		t.Errorf("Read of an absent type = %q, want nil", missing)
	}
	if afterClear != nil {
		t.Errorf("ReadTypes after Write(nil) = %q, want nil", afterClear)
	}
}

func TestLiveClipboardReadsBitmaps(t *testing.T) {
	requireLiveIntegration(t)
	img := testImage()
	var got []byte
	clipboardSession(t, func(b *Backend, quit func()) {
		// A program that copies a bitmap only (Paint, a screenshot tool).
		h, err := globalFromBytes(dibV5(img))
		if err != nil {
			t.Fatal(err)
		}
		if err := b.openClipboard(); err != nil {
			t.Fatal(err)
		}
		procEmptyClipboard.Call()
		procSetClipboardData.Call(cfDIBV5, h)
		procCloseClipboard.Call()
		b.Clipboard().Read(platform.MIMEPNG, func(b []byte, err error) {
			if err != nil {
				t.Errorf("Read(PNG): %v", err)
			}
			got = b
			quit()
		})
	})
	checkPNGPixels(t, got, img)
}

func TestLiveSetMenu(t *testing.T) {
	requireLiveIntegration(t)
	b := liveBackend(t)
	var framed, frameless *Window
	var menuEvents []platform.MenuItemID
	step := 0
	menu := []platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{
			{ID: 100, Label: "Open", Accelerator: "Ctrl+O"},
			{Separator: true},
			{ID: 101, Label: "Disabled", Disabled: true},
			{ID: 102, Label: "Checked", Checkable: true, Checked: true},
		}},
		{ID: 200, Label: "Help & about"},
	}
	runLive(t, b, func() {
		a, err := b.NewWindow(platform.WindowOptions{Title: "tela menu", Size: platform.Size{W: 320, H: 200}})
		if err != nil {
			t.Fatal(err)
		}
		c, err := b.NewWindow(platform.WindowOptions{Title: "tela frameless", Size: platform.Size{W: 120, H: 80}, Frameless: true})
		if err != nil {
			t.Fatal(err)
		}
		framed, frameless = a.(*Window), c.(*Window)
	}, func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, solidBlack)
			if w != platform.Window(framed) || step != 0 {
				return
			}
			step = 1
			if err := frameless.SetMenu(menu); !errors.Is(err, platform.ErrUnsupported) {
				t.Errorf("SetMenu on a frameless window = %v, want ErrUnsupported", err)
			}
			before := getClientRect(framed.hwnd)
			if err := framed.SetMenu(menu); err != nil {
				t.Errorf("SetMenu: %v", err)
				b.Quit()
				return
			}
			bar, _, _ := procGetMenu.Call(framed.hwnd)
			if bar == 0 {
				t.Errorf("the window has no menu bar")
				b.Quit()
				return
			}
			if after := getClientRect(framed.hwnd); after != before {
				t.Errorf("client area %v after SetMenu, want %v (the window grows)", after, before)
			}
			checkMenuString(t, bar, 1, "Help && about")
			if n, _, _ := procGetMenuItemCount.Call(bar); n != 2 {
				t.Errorf("menu bar has %d items, want 2", n)
			}
			sub, _, _ := procGetSubMenu.Call(bar, 0)
			checkMenuString(t, sub, 0, "Open\tCtrl+O")
			const mfGrayedDisabled = mfGrayed | 0x0002
			if st, _, _ := procGetMenuState.Call(sub, 2, mfByPosition); st&mfGrayedDisabled == 0 {
				t.Errorf("disabled item state %#x", st)
			}
			if st, _, _ := procGetMenuState.Call(sub, 3, mfByPosition); st&mfChecked == 0 {
				t.Errorf("checked item state %#x", st)
			}
			// The menu sends WM_COMMAND with the item's command ID.
			postMessage(framed.hwnd, wmCommand, 1, 0) // Open
			postMessage(framed.hwnd, wmCommand, 4, 0) // Help & about
		case platform.MenuEvent:
			menuEvents = append(menuEvents, e.Item)
			if len(menuEvents) == 2 {
				before := getClientRect(framed.hwnd)
				if err := framed.SetMenu(nil); err != nil {
					t.Errorf("SetMenu(nil): %v", err)
				}
				if bar, _, _ := procGetMenu.Call(framed.hwnd); bar != 0 {
					t.Errorf("SetMenu(nil) left a menu bar")
				}
				if after := getClientRect(framed.hwnd); after != before {
					t.Errorf("client area %v after removing the menu, want %v", after, before)
				}
				b.Quit()
			}
		}
	})
	if !slices.Equal(menuEvents, []platform.MenuItemID{100, 200}) {
		t.Errorf("MenuEvents = %v, want [100 200]", menuEvents)
	}
}

func checkMenuString(t *testing.T, menu uintptr, pos int, want string) {
	t.Helper()
	var buf [128]uint16
	n, _, _ := procGetMenuStringW.Call(menu, uintptr(pos), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), mfByPosition)
	if got := fromUTF16Clipboard(buf[:n]); got != want {
		t.Errorf("menu item %d = %q, want %q", pos, got, want)
	}
}

func TestLivePopupMenu(t *testing.T) {
	requireLiveIntegration(t)
	b := liveBackend(t)
	var answers []platform.MenuItemID
	started := false
	menu := []platform.MenuItem{
		{ID: 5, Label: "First", Accelerator: "Ctrl+1"},
		{ID: 6, Label: "Disabled", Disabled: true},
		{Separator: true},
		{Label: "More", Submenu: []platform.MenuItem{{ID: 7, Label: "Inner"}}},
	}
	// keys answers the popup menu that opens next with key taps, posted
	// to the menu's window: SendInput goes to the foreground window, and
	// a test started from a scheduled task may not own the foreground
	// (the menu then waited forever). A menu still open after 5 s is
	// closed with Escape and the test fails.
	var stuck atomic.Bool
	keys := func(vks ...uintptr) {
		go func() {
			menu := waitWindow("#32768", "")
			if menu == 0 {
				return // the test fails on the timeout
			}
			time.Sleep(150 * time.Millisecond)
			for _, vk := range vks {
				procPostMessageW.Call(menu, 0x0100, vk, 1)          // WM_KEYDOWN
				procPostMessageW.Call(menu, 0x0101, vk, 0xC0000001) // WM_KEYUP
			}
			for deadline := time.Now().Add(5 * time.Second); isWindow(menu); time.Sleep(50 * time.Millisecond) {
				if time.Now().After(deadline) {
					stuck.Store(true)
					procPostMessageW.Call(menu, 0x0100, 0x1B, 1) // VK_ESCAPE
					return
				}
			}
		}()
	}
	runLive(t, b, func() {
		if _, err := b.NewWindow(platform.WindowOptions{Title: "tela popup", Size: platform.Size{W: 320, H: 200}}); err != nil {
			t.Fatal(err)
		}
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		presentSolid(t, w, solidBlack)
		if started {
			return
		}
		started = true
		inCall := true
		err := w.PopupMenu(platform.Point{X: 20, Y: 20}, menu, func(item platform.MenuItemID, err error) {
			if inCall {
				t.Errorf("PopupMenu answered synchronously")
			}
			if err != nil {
				t.Errorf("PopupMenu: %v", err)
			}
			answers = append(answers, item)
			// Escape dismisses the second one.
			keys(0x1B) // VK_ESCAPE
			w.PopupMenu(platform.Point{X: 40, Y: 40}, menu, func(item platform.MenuItemID, err error) {
				answers = append(answers, item)
				b.Quit()
			})
		})
		inCall = false
		if err != nil {
			t.Errorf("PopupMenu: %v", err)
			b.Quit()
			return
		}
		keys(0x28, 0x0D) // VK_DOWN, VK_RETURN: the first item
	})
	if stuck.Load() {
		t.Error("a popup menu ignored the posted keys for 5 s and was closed with Escape")
	}
	if !slices.Equal(answers, []platform.MenuItemID{5, 0}) {
		t.Errorf("popup answers = %v, want [5 0]", answers)
	}
}
