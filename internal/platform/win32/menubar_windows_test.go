//go:build windows

package win32

import (
	"testing"
	"unsafe"
)

// The structures of the dark menu bar against the sizes and offsets of the
// C declarations, on 32 and 64-bit Windows.
func TestMenuBarStructLayouts(t *testing.T) {
	w64 := unsafe.Sizeof(uintptr(0)) == 8
	pick := func(a, b uintptr) uintptr {
		if w64 {
			return a
		}
		return b
	}
	var dis drawItemStruct
	var u uahDrawMenuItem
	var mii menuItemInfo
	var mbi menuBarInfo
	var ncm nonClientMetrics
	var gti guiThreadInfo
	for name, tc := range map[string]struct{ got, want uintptr }{
		"DRAWITEMSTRUCT size":       {unsafe.Sizeof(dis), pick(64, 48)},
		"DRAWITEMSTRUCT itemState":  {unsafe.Offsetof(dis.itemState), 16},
		"DRAWITEMSTRUCT rcItem":     {unsafe.Offsetof(dis.rcItem), pick(40, 28)},
		"UAHMENU size":              {unsafe.Sizeof(uahMenu{}), pick(24, 12)},
		"UAHDRAWMENUITEM um":        {unsafe.Offsetof(u.um), pick(64, 48)},
		"UAHDRAWMENUITEM iPosition": {unsafe.Offsetof(u.position), pick(88, 60)},
		"MENUITEMINFOW size":        {unsafe.Sizeof(mii), pick(80, 48)},
		"MENUITEMINFOW dwTypeData":  {unsafe.Offsetof(mii.typeData), pick(56, 36)},
		"MENUITEMINFOW cch":         {unsafe.Offsetof(mii.cch), pick(64, 40)},
		"MENUBARINFO size":          {unsafe.Sizeof(mbi), pick(48, 32)},
		"MENUBARINFO hMenu":         {unsafe.Offsetof(mbi.menu), pick(24, 20)},
		"LOGFONTW size":             {unsafe.Sizeof(logFont{}), 92},
		"NONCLIENTMETRICSW size":    {unsafe.Sizeof(ncm), 504},
		"NONCLIENTMETRICSW menu":    {unsafe.Offsetof(ncm.menuFont), 24 + 92 + 8 + 92 + 8},
		"GUITHREADINFO size":        {unsafe.Sizeof(gti), pick(72, 48)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", name, tc.got, tc.want)
		}
	}
}
