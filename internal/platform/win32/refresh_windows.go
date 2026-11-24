//go:build windows

package win32

import (
	"time"
	"unsafe"
)

// FrameEvent.Refresh: the refresh rate of the monitor a window is on.
//
// The monitor is MonitorFromWindow's, the one the largest part of the
// window is on (the nearest for a window off every monitor), and the rate
// its current mode's dmDisplayFrequency, as winit reads it. A window that
// moved is seen at its next frame, since the monitor is asked for each; the
// rates are kept per monitor, and read again after WM_DISPLAYCHANGE, which
// every top-level window gets when a mode or the set of monitors changes
// (monitor handles are not valid across it). Windows 11's dynamic refresh
// rate switches without a mode change: the mode's rate is its highest.

// refreshHook is the loop's Refresh hook.
func (b *Backend) refreshHook(w *Window) time.Duration {
	mon := monitorFromWindow(w.hwnd)
	if mon == 0 {
		return 0
	}
	d, ok := b.refreshes[mon]
	if !ok {
		d = monitorRefresh(mon)
		if b.refreshes == nil {
			b.refreshes = map[uintptr]time.Duration{}
		}
		b.refreshes[mon] = d
	}
	return d
}

// monitorRefresh returns the refresh interval of a monitor, 0 when it
// cannot be read.
func monitorRefresh(mon uintptr) time.Duration {
	var mi monitorInfoEx
	mi.size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return 0
	}
	dm := devMode{size: uint16(unsafe.Sizeof(devMode{}))}
	r, _, _ := procEnumDisplaySetting.Call(uintptr(unsafe.Pointer(&mi.device[0])), uintptr(enumCurrentSets), uintptr(unsafe.Pointer(&dm)))
	if r == 0 {
		return 0
	}
	return refreshOf(dm.displayFrequency)
}
