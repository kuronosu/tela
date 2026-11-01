//go:build windows

package win32

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// App-level services of phase 13 (ADR-209, ADR-210): global shortcuts
// (hotkey.go), secrets (secret.go), single instance (instance.go) and the
// settings of v2 (settings.go, systemsettings.go). The files with the
// _windows suffix make the OS calls; the others are pure and tested on
// every OS.

// Procedures of the app-level services of phase 13. Kept in a block of
// their own, apart from procs.go, so that the services other agents add
// to the backend do not collide with these lines.
var (
	advapi32 = windows.NewLazySystemDLL("advapi32.dll") // Credential Manager (ADR-209)

	procRegisterHotKey           = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	procVkKeyScanExW             = user32.NewProc("VkKeyScanExW")
	procFindWindowExW            = user32.NewProc("FindWindowExW")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procSendMessageTimeoutW      = user32.NewProc("SendMessageTimeoutW")

	procDwmGetColorizationColor = dwmapi.NewProc("DwmGetColorizationColor")

	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

// appServices is the state of the app-level services, on the UI thread.
type appServices struct {
	// Global shortcuts.
	shortcuts map[string]*hotkeyReg // by shortcut ID
	byHotKey  map[int32]string      // hotkey ID -> shortcut ID
	hotkeyIDs hotkeyIDs

	// Single instance: the app ID this process claimed, and the mutex that
	// holds the claim until Run returns.
	instanceID    string
	instanceMutex windows.Handle
}

// hotkeyReg is a shortcut asked for with RegisterShortcut.
type hotkeyReg struct {
	id         int32 // the RegisterHotKey ID
	registered bool  // RegisterHotKey succeeded
}

// appServicesMessage handles the messages of the app-level services that
// reach the backend's own windows (the message-only window and the tray
// window); ok is false for any other.
func (b *Backend) appServicesMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) (ret uintptr, ok bool) {
	switch msg {
	case wmHotKey:
		if hwnd == b.msgHwnd {
			b.onHotKey(int32(wparam))
			return 0, true
		}
	case wmCopyData:
		if hwnd == b.msgHwnd {
			return b.onCopyData(lparam), true
		}
	case wmSettingChange, wmDwmColorizationColorChanged, wmSysColorChange:
		// Top-level windows get these; so does the tray window, which
		// outlives the app's windows.
		if hwnd == b.desk.hwnd && hwnd != 0 {
			b.settingsChanged()
			b.notify()
			return 0, true
		}
	}
	return 0, false
}

// appServicesShutdown releases the app-level services when Run returns:
// the hotkeys are unregistered and the single instance claim is given up.
func (b *Backend) appServicesShutdown() {
	a := &b.app
	for id, h := range a.shortcuts {
		if h.registered {
			unregisterHotKey(b.msgHwnd, h.id)
		}
		delete(a.shortcuts, id)
	}
	clear(a.byHotKey)
	if a.instanceMutex != 0 {
		_ = windows.CloseHandle(a.instanceMutex)
		a.instanceMutex, a.instanceID = 0, ""
	}
}

// --- global shortcuts ---

func (d desktop) RegisterShortcut(s platform.Shortcut) error {
	platform.CheckShortcut(s)
	if d.b.closed {
		return errClosed
	}
	d.b.registerShortcut(s)
	return nil
}

func (d desktop) UnregisterShortcut(id string) error {
	platform.CheckShortcutID("UnregisterShortcut", id)
	d.b.unregisterShortcut(id)
	return nil
}

// registerShortcut binds s with RegisterHotKey on the message-only window
// and queues the ShortcutStatusEvent with the outcome. Registering an ID
// again replaces its binding: the old hotkey is released first, so asking
// for the same combination again works.
func (b *Backend) registerShortcut(s platform.Shortcut) {
	a := &b.app
	b.unregisterShortcut(s.ID)
	report := func(status platform.ShortcutStatus, reason string) {
		ev := platform.ShortcutStatusEvent{ID: s.ID, Status: status, Reason: reason}
		if status == platform.ShortcutBound {
			ev.Trigger = s.Accelerator.String()
		}
		b.enqueue(nil, ev)
		b.notify()
	}
	vk, err := hotkeyVK(s.Accelerator.Key, scanKey)
	if err != nil {
		report(platform.ShortcutFailed, err.Error())
		return
	}
	id, ok := a.hotkeyIDs.alloc()
	if !ok {
		report(platform.ShortcutFailed, "all 49151 hotkey IDs of the process are in use")
		return
	}
	r, _, e := procRegisterHotKey.Call(b.msgHwnd, uintptr(id), uintptr(hotkeyModifiers(s.Accelerator.Mods)), uintptr(vk))
	if r == 0 {
		a.hotkeyIDs.free(id)
		if errno, _ := e.(syscall.Errno); errno == errHotKeyAlreadyRegistered {
			report(platform.ShortcutTaken, fmt.Sprintf("another application has registered %s as a hotkey", s.Accelerator))
			return
		}
		report(platform.ShortcutFailed, winError("RegisterHotKey", e).Error())
		return
	}
	if a.shortcuts == nil {
		a.shortcuts, a.byHotKey = map[string]*hotkeyReg{}, map[int32]string{}
	}
	a.shortcuts[s.ID] = &hotkeyReg{id: id, registered: true}
	a.byHotKey[id] = s.ID
	report(platform.ShortcutBound, "")
}

// unregisterShortcut releases the hotkey of a shortcut; an unknown ID is
// ignored.
func (b *Backend) unregisterShortcut(id string) {
	a := &b.app
	h := a.shortcuts[id]
	if h == nil {
		return
	}
	if h.registered && !b.stopped.Load() {
		unregisterHotKey(b.msgHwnd, h.id)
	}
	delete(a.shortcuts, id)
	delete(a.byHotKey, h.id)
	a.hotkeyIDs.free(h.id)
}

func unregisterHotKey(hwnd uintptr, id int32) {
	procUnregisterHotKey.Call(hwnd, uintptr(id))
}

// onHotKey handles WM_HOTKEY: Windows reports presses only.
func (b *Backend) onHotKey(id int32) {
	name, ok := b.app.byHotKey[id]
	if !ok {
		return
	}
	b.enqueue(nil, platform.ShortcutEvent{ID: name, Action: platform.ShortcutPressed, Time: b.Now()})
	b.notify()
}

// scanKey is VkKeyScanExW for the keyboard layout of the UI thread: the
// virtual key of a character; ok is false when the layout has none.
func scanKey(r rune) (vk uint32, ok bool) {
	if r > 0xFFFF {
		return 0, false
	}
	res, _, _ := procVkKeyScanExW.Call(uintptr(r), currentLayout())
	if int16(res) == -1 || uint8(res) == 0xFF {
		return 0, false
	}
	return uint32(uint8(res)), true
}

// --- secrets ---

func (d desktop) SetSecret(appID, name string, secret []byte) error {
	if err := platform.CheckSecret(appID, name, secret); err != nil {
		return err
	}
	if !available(procCredWriteW) {
		return platform.ErrUnsupported
	}
	cred := credential{
		typ:        credTypeGeneric,
		targetName: utf16(credentialTarget(appID, name)),
		persist:    credPersistLocalMachine,
		userName:   utf16(appID),
	}
	if len(secret) > 0 {
		cred.credentialBlob = &secret[0]
		cred.credentialBlobSize = uint32(len(secret))
	}
	r, _, e := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if r == 0 {
		return secretError(fmt.Sprintf("SetSecret(%q, %q)", appID, name), e)
	}
	return nil
}

func (d desktop) Secret(appID, name string) ([]byte, error) {
	platform.CheckSecretName("Secret", appID, name)
	if !available(procCredReadW) {
		return nil, platform.ErrUnsupported
	}
	var p *credential
	r, _, e := procCredReadW.Call(uintptr(unsafe.Pointer(utf16(credentialTarget(appID, name)))), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return nil, secretError(fmt.Sprintf("Secret(%q, %q)", appID, name), e)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(p)))
	secret := make([]byte, p.credentialBlobSize)
	if p.credentialBlobSize > 0 {
		copy(secret, unsafe.Slice(p.credentialBlob, p.credentialBlobSize))
	}
	return secret, nil
}

func (d desktop) DeleteSecret(appID, name string) error {
	platform.CheckSecretName("DeleteSecret", appID, name)
	if !available(procCredDeleteW) {
		return platform.ErrUnsupported
	}
	r, _, e := procCredDeleteW.Call(uintptr(unsafe.Pointer(utf16(credentialTarget(appID, name)))), credTypeGeneric, 0)
	if r == 0 {
		if err := secretError(fmt.Sprintf("DeleteSecret(%q, %q)", appID, name), e); !errors.Is(err, platform.ErrSecretNotFound) {
			return err
		}
	}
	return nil
}

// --- single instance ---

const (
	// instanceFindTimeout is how long a second process looks for the
	// first's window: the first creates the mutex a moment before its
	// window has the name.
	instanceFindTimeout = 3 * time.Second
	// instanceSendTimeout (ms) is how long it waits for the first to take
	// the launch.
	instanceSendTimeout = 5000
	smtoBlock           = 0x0001
	smtoAbortIfHung     = 0x0002
)

func (d desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	platform.CheckAppID("ClaimInstance", appID)
	return d.b.claimInstance(appID, launch)
}

// claimInstance creates the named mutex. The process that creates it is
// the first: it names its message-only window after the app ID, so that
// later processes find it. A process that finds the mutex taken forwards
// its launch to the first and is not.
func (b *Backend) claimInstance(appID string, launch platform.Launch) (primary bool, err error) {
	a := &b.app
	if b.closed {
		return false, errClosed
	}
	if a.instanceMutex != 0 {
		if a.instanceID == appID {
			return true, nil
		}
		return false, fmt.Errorf("win32: ClaimInstance(%q): this process already holds the claim of %q", appID, a.instanceID)
	}
	name, err := syscall.UTF16PtrFromString(instanceMutexName(appID))
	if err != nil {
		return false, fmt.Errorf("win32: ClaimInstance(%q): %w", appID, err)
	}
	h, err := windows.CreateMutex(nil, false, name)
	switch {
	case h == 0:
		return false, winError("CreateMutexW", err)
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
		_ = windows.CloseHandle(h)
		if err := b.forwardLaunch(appID, launch); err != nil {
			return false, err
		}
		return false, nil
	}
	a.instanceMutex, a.instanceID = h, appID
	procSetWindowTextW.Call(b.msgHwnd, uintptr(unsafe.Pointer(utf16(instanceWindowName(appID)))))
	return true, nil
}

// forwardLaunch sends launch to the first process of the app: it finds
// its message-only window, lets that process take the foreground
// (AllowSetForegroundWindow: this process was just started by the user and
// may), and sends the launch with WM_COPYDATA, which returns when the
// first has queued it.
func (b *Backend) forwardLaunch(appID string, launch platform.Launch) error {
	class := unsafe.Pointer(classNamePtr)
	title := utf16(instanceWindowName(appID))
	var target uintptr
	for deadline := time.Now().Add(instanceFindTimeout); ; time.Sleep(25 * time.Millisecond) {
		target, _, _ = procFindWindowExW.Call(hwndMessage, 0, uintptr(class), uintptr(unsafe.Pointer(title)))
		if target != 0 || time.Now().After(deadline) {
			break
		}
	}
	if target == 0 {
		return fmt.Errorf("win32: ClaimInstance(%q): another process holds the claim, but its window was not found within %v", appID, instanceFindTimeout)
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(target, uintptr(unsafe.Pointer(&pid)))
	procAllowSetForegroundWindow.Call(uintptr(pid))

	payload := encodeLaunch(launch)
	cds := copyDataStruct{dwData: copyDataLaunch, cbData: uint32(len(payload)), lpData: uintptr(unsafe.Pointer(&payload[0]))}
	var result uintptr
	r, _, e := procSendMessageTimeoutW.Call(target, wmCopyData, b.msgHwnd, uintptr(unsafe.Pointer(&cds)),
		smtoBlock|smtoAbortIfHung, instanceSendTimeout, uintptr(unsafe.Pointer(&result)))
	runtime.KeepAlive(payload)
	switch {
	case r == 0:
		return winError(fmt.Sprintf("sending the launch to the first instance of %q", appID), e)
	case result == 0:
		return fmt.Errorf("win32: ClaimInstance(%q): the first instance did not accept the launch", appID)
	}
	return nil
}

// onCopyData handles WM_COPYDATA: a launch forwarded by a second process
// becomes an app-scoped LaunchEvent. The message is answered TRUE when the
// launch was taken. The data is only valid during the call, so it is
// decoded (and copied) here.
func (b *Backend) onCopyData(lparam uintptr) uintptr {
	if b.app.instanceMutex == 0 || lparam == 0 {
		return 0
	}
	cds := (*copyDataStruct)(ptrFromUintptr(lparam))
	if cds.dwData != copyDataLaunch || cds.cbData == 0 || cds.cbData > maxLaunchPayload || cds.lpData == 0 {
		return 0
	}
	launch, err := decodeLaunch(unsafe.Slice((*byte)(ptrFromUintptr(cds.lpData)), cds.cbData))
	if err != nil {
		return 0
	}
	b.enqueue(nil, platform.LaunchEvent{Launch: launch})
	b.notify()
	return 1
}
