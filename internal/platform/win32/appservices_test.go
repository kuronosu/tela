package win32

import (
	"errors"
	"image/color"
	"io/fs"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure logic of the app-level services (systemsettings.go, hotkey.go,
// instance.go, secret.go), tested on every OS; the live tests run the real
// calls on Windows.

func TestSettingsFrom(t *testing.T) {
	base := rawSettings{AppsUseLightTheme: 1, ThemeKnown: true, ClientAreaAnimation: true, DoubleClickMs: 500}
	with := func(f func(*rawSettings)) rawSettings { r := base; f(&r); return r }
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	for _, tc := range []struct {
		name string
		in   rawSettings
		want platform.SystemSettings
	}{
		{"light", base, platform.SystemSettings{DoubleClickInterval: ms(500), TextScale: 1}},
		{"dark", with(func(r *rawSettings) { r.AppsUseLightTheme = 0; r.DoubleClickMs = 400 }),
			platform.SystemSettings{DarkMode: true, DoubleClickInterval: ms(400), TextScale: 1}},
		{"theme value missing is light", with(func(r *rawSettings) { r.AppsUseLightTheme, r.ThemeKnown = 0, false }),
			platform.SystemSettings{DoubleClickInterval: ms(500), TextScale: 1}},
		{"animations off is reduced motion", with(func(r *rawSettings) { r.ClientAreaAnimation = false }),
			platform.SystemSettings{ReducedMotion: true, DoubleClickInterval: ms(500), TextScale: 1}},
		{"any non-zero is light", with(func(r *rawSettings) { r.AppsUseLightTheme, r.DoubleClickMs = 7, 0 }),
			platform.SystemSettings{TextScale: 1}},
		{"accent", with(func(r *rawSettings) { r.Accent, r.HasAccent = color.NRGBA{R: 0, G: 120, B: 212, A: 255}, true }),
			platform.SystemSettings{DoubleClickInterval: ms(500), TextScale: 1, AccentColor: color.NRGBA{R: 0, G: 120, B: 212, A: 255}, HasAccentColor: true}},
		{"high contrast", with(func(r *rawSettings) { r.HighContrast = true }),
			platform.SystemSettings{DoubleClickInterval: ms(500), TextScale: 1, HighContrast: true}},
		{"text scale", with(func(r *rawSettings) { r.TextScalePercent = 150 }),
			platform.SystemSettings{DoubleClickInterval: ms(500), TextScale: 1.5}},
	} {
		got := settingsFrom(tc.in)
		if got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
		platform.CheckSystemSettings(got)
	}
}

func TestTextScaleFrom(t *testing.T) {
	for in, want := range map[uint32]float64{0: 1, 99: 1, 100: 1, 125: 1.25, 225: 2.25, 226: 2.25, 1000: 2.25} {
		if got := textScaleFrom(in); got != want {
			t.Errorf("textScaleFrom(%d) = %v, want %v", in, got, want)
		}
	}
}

func TestAccentConversions(t *testing.T) {
	// The Windows 11 default blue, #0078D4: ABGR in DWM\AccentColor, ARGB
	// from DwmGetColorizationColor.
	want := color.NRGBA{R: 0x00, G: 0x78, B: 0xD4, A: 0xFF}
	if got := accentFromABGR(0xFFD47800); got != want {
		t.Errorf("accentFromABGR = %v, want %v", got, want)
	}
	if got := accentFromARGB(0xC40078D4); got != want {
		t.Errorf("accentFromARGB (alpha is not the accent's) = %v, want %v", got, want)
	}
	if got := accentFromABGR(0x0000FF00); got.A != 0xFF || got.G != 0xFF {
		t.Errorf("accentFromABGR ignores the alpha byte: %v", got)
	}
}

func TestStructLayouts(t *testing.T) {
	w64 := unsafe.Sizeof(uintptr(0)) == 8
	pick := func(a, b uintptr) uintptr {
		if w64 {
			return a
		}
		return b
	}
	var hc highContrast
	var cred credential
	var cds copyDataStruct
	for name, tc := range map[string]struct{ got, want uintptr }{
		"HIGHCONTRASTW size":         {unsafe.Sizeof(hc), pick(16, 12)},
		"HIGHCONTRASTW cbSize":       {unsafe.Offsetof(hc.size), 0},
		"HIGHCONTRASTW dwFlags":      {unsafe.Offsetof(hc.flags), 4},
		"CREDENTIALW Flags":          {unsafe.Offsetof(cred.flags), 0},
		"CREDENTIALW Type":           {unsafe.Offsetof(cred.typ), 4},
		"COPYDATASTRUCT dwData":      {unsafe.Offsetof(cds.dwData), 0},
		"HIGHCONTRASTW lpszDefault":  {unsafe.Offsetof(hc.defaultScheme), pick(8, 8)},
		"CREDENTIALW size":           {unsafe.Sizeof(cred), pick(80, 52)},
		"CREDENTIALW TargetName":     {unsafe.Offsetof(cred.targetName), 8},
		"CREDENTIALW LastWritten":    {unsafe.Offsetof(cred.lastWritten), pick(24, 16)},
		"CREDENTIALW BlobSize":       {unsafe.Offsetof(cred.credentialBlobSize), pick(32, 24)},
		"CREDENTIALW Blob":           {unsafe.Offsetof(cred.credentialBlob), pick(40, 28)},
		"CREDENTIALW Persist":        {unsafe.Offsetof(cred.persist), pick(48, 32)},
		"CREDENTIALW Attributes":     {unsafe.Offsetof(cred.attributes), pick(56, 40)},
		"CREDENTIALW UserName":       {unsafe.Offsetof(cred.userName), pick(72, 48)},
		"COPYDATASTRUCT size":        {unsafe.Sizeof(cds), pick(24, 12)},
		"COPYDATASTRUCT cbData":      {unsafe.Offsetof(cds.cbData), pick(8, 4)},
		"COPYDATASTRUCT lpData":      {unsafe.Offsetof(cds.lpData), pick(16, 8)},
		"CREDENTIALW Comment":        {unsafe.Offsetof(cred.comment), pick(16, 12)},
		"CREDENTIALW TargetAlias":    {unsafe.Offsetof(cred.targetAlias), pick(64, 44)},
		"CREDENTIALW AttributeCount": {unsafe.Offsetof(cred.attributeCount), pick(52, 36)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", name, tc.got, tc.want)
		}
	}
}

func TestHotkeyModifiers(t *testing.T) {
	for _, tc := range []struct {
		in   platform.Modifiers
		want uint32
	}{
		{0, modNoRepeat},
		{platform.ModCtrl | platform.ModShift, modNoRepeat | modControl | modShift},
		{platform.ModAlt, modNoRepeat | modAlt},
		{platform.ModSuper | platform.ModCtrl | platform.ModAlt | platform.ModShift, modNoRepeat | modWin | modControl | modAlt | modShift},
	} {
		if got := hotkeyModifiers(tc.in); got != tc.want {
			t.Errorf("hotkeyModifiers(%v) = %#x, want %#x", tc.in, got, tc.want)
		}
	}
}

func TestHotkeyVK(t *testing.T) {
	// A layout where the only keys it knows are these: the scan function
	// is VkKeyScanExW.
	scan := func(r rune) (uint32, bool) {
		switch r {
		case 'k':
			return 'K', true
		case '1': // AZERTY: the digit needs Shift, the key is VK_1 all the same
			return '1', true
		case '+':
			return 0xBB, true
		}
		return 0, false
	}
	for _, tc := range []struct {
		key  platform.Key
		scan func(rune) (uint32, bool)
		want uint32
		err  string
	}{
		{"k", scan, 'K', ""},
		{"1", scan, '1', ""},
		{"+", scan, 0xBB, ""},
		{"k", nil, 'K', ""},  // letters fall back to their own virtual key
		{"z", scan, 'Z', ""}, // the layout has no 'z': the fallback
		{"7", nil, '7', ""},
		{" ", nil, vkSpace, ""},
		{"/", nil, 0xBF, ""}, // US punctuation
		{"é", scan, 0, "not on the current keyboard layout"},
		{platform.KeyF1, nil, vkF1, ""},
		{platform.KeyF12, nil, vkF1 + 11, ""},
		{"F24", nil, 0x87, ""},
		{platform.KeyEscape, nil, vkEscape, ""},
		{platform.KeyEnter, nil, vkReturn, ""},
		{platform.KeyArrowUp, nil, vkUp, ""},
		{platform.KeyPageDown, nil, vkNext, ""},
		{platform.KeyDelete, nil, vkDelete, ""},
		{platform.KeyClear, nil, vkClear, ""},
		{platform.KeyMediaPlayPause, nil, 0xB3, ""},
		{platform.KeyDead, nil, 0, "no virtual key"},
	} {
		got, err := hotkeyVK(tc.key, tc.scan)
		switch {
		case tc.err != "":
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("hotkeyVK(%q) error = %v, want one with %q", tc.key, err, tc.err)
			}
		case err != nil || got != tc.want:
			t.Errorf("hotkeyVK(%q) = %#x, %v; want %#x", tc.key, got, err, tc.want)
		}
	}
}

func TestHotkeyIDs(t *testing.T) {
	var h hotkeyIDs
	a, _ := h.alloc()
	b, _ := h.alloc()
	if a != 1 || b != 2 {
		t.Fatalf("first IDs = %d, %d", a, b)
	}
	h.free(a)
	if c, _ := h.alloc(); c != 3 {
		t.Errorf("an ID is reused only after the range wraps; got %d", c)
	}
	// Fill the range: the last ID is maxHotKeyID, then the free one is found.
	h = hotkeyIDs{last: maxHotKeyID - 1}
	if id, ok := h.alloc(); !ok || id != maxHotKeyID {
		t.Fatalf("alloc at the end = %d, %v", id, ok)
	}
	if id, ok := h.alloc(); !ok || id != 1 {
		t.Fatalf("alloc after wrapping = %d, %v", id, ok)
	}
	h = hotkeyIDs{}
	for range maxHotKeyID {
		if _, ok := h.alloc(); !ok {
			t.Fatal("alloc failed before the range was used up")
		}
	}
	if id, ok := h.alloc(); ok {
		t.Errorf("alloc after the range was used up = %d", id)
	}
	h.free(500)
	if id, ok := h.alloc(); !ok || id != 500 {
		t.Errorf("alloc after free(500) = %d, %v", id, ok)
	}
}

func TestInstanceNames(t *testing.T) {
	if got := instanceMutexName("com.example.Notes"); got != `Local\tela-com.example.Notes` {
		t.Errorf("mutex name = %q", got)
	}
	if got := instanceWindowName("com.example.Notes"); got != "tela-instance-com.example.Notes" {
		t.Errorf("window name = %q", got)
	}
	// Anything outside the safe set is replaced, and the hash keeps two
	// different IDs apart.
	a, b := sanitizedAppID("app:one"), sanitizedAppID("app;one")
	if a == b || !strings.HasPrefix(a, "app_one-") || !strings.HasPrefix(b, "app_one-") {
		t.Errorf("sanitizedAppID = %q, %q", a, b)
	}
	if a != sanitizedAppID("app:one") {
		t.Errorf("sanitizedAppID is not deterministic")
	}
	long := strings.Repeat("x", 255)
	if n := sanitizedAppID(long); len(n) > maxIDName+17 || strings.Contains(n, `\`) {
		t.Errorf("a long ID gave a name of %d bytes", len(n))
	}
	if n := len(instanceMutexName(long)); n >= 260 {
		t.Errorf("mutex name of %d characters does not fit MAX_PATH", n)
	}
	if sanitizedAppID("ünï") == sanitizedAppID("ünï2") {
		t.Errorf("non-ASCII IDs collide")
	}
}

func TestLaunchPayload(t *testing.T) {
	for _, l := range []platform.Launch{
		{},
		{Dir: `C:\Users\u\notes`, Args: []string{"notes:open?id=3"}},
		{Dir: "/home/u", Token: "tok", Args: []string{"", "a b", "ünï", "x\x00y", strings.Repeat("z", 5000)}},
	} {
		got, err := decodeLaunch(encodeLaunch(l))
		if err != nil {
			t.Errorf("decodeLaunch(%+v): %v", l, err)
			continue
		}
		if got.Dir != l.Dir || got.Token != l.Token || !slices.Equal(got.Args, l.Args) {
			t.Errorf("round trip of %+v gave %+v", l, got)
		}
	}
	good := encodeLaunch(platform.Launch{Dir: "d", Token: "t", Args: []string{"a", "bc"}})
	// Every truncation and a trailing byte are refused without a panic.
	for n := range good {
		if _, err := decodeLaunch(good[:n]); err == nil {
			t.Errorf("a payload cut at %d of %d bytes was accepted", n, len(good))
		}
	}
	if _, err := decodeLaunch(append(slices.Clone(good), 0)); err == nil {
		t.Errorf("a payload with a trailing byte was accepted")
	}
	// A count far larger than the payload must not allocate or loop.
	if _, err := decodeLaunch([]byte{launchVersion, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}); err == nil {
		t.Errorf("a huge argument count was accepted")
	}
	if _, err := decodeLaunch([]byte{launchVersion + 1, 0, 0, 0}); err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("another version: %v", err)
	}
	if _, err := decodeLaunch(nil); err == nil {
		t.Errorf("an empty payload was accepted")
	}
}

func TestCredentialTarget(t *testing.T) {
	if got := credentialTarget("com.example.Notes", "key/1"); got != "com.example.Notes/key/1" {
		t.Errorf("credentialTarget = %q", got)
	}
}

// The numbers the OS calls take; a typo in one is found only on Windows.
func TestServiceConstants(t *testing.T) {
	for name, tc := range map[string]struct{ got, want uint32 }{
		"WM_HOTKEY":                       {wmHotKey, 0x0312},
		"WM_COPYDATA":                     {wmCopyData, 0x004A},
		"WM_SYSCOLORCHANGE":               {wmSysColorChange, 0x0015},
		"WM_DWMCOLORIZATIONCOLORCHANGED":  {wmDwmColorizationColorChanged, 0x0320},
		"ERROR_HOTKEY_ALREADY_REGISTERED": {errHotKeyAlreadyRegistered, 1409},
		"SPI_GETHIGHCONTRAST":             {spiGetHighContrast, 0x0042},
		"HCF_HIGHCONTRASTON":              {hcfHighContrastOn, 1},
		"CRED_TYPE_GENERIC":               {credTypeGeneric, 1},
		"CRED_PERSIST_LOCAL_MACHINE":      {credPersistLocalMachine, 2},
		"ERROR_NOT_FOUND":                 {uint32(errorNotFound), 1168},
		"ERROR_NO_SUCH_LOGON_SESSION":     {uint32(errorNoSuchLogonSession), 1312},
		"MOD_NOREPEAT":                    {modNoRepeat, 0x4000},
		"copy data launch tag":            {copyDataLaunch, 0x544C4101},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %#x, want %#x", name, tc.got, tc.want)
		}
	}
}

func TestSecretError(t *testing.T) {
	if err := secretError("Secret", errorNotFound); !errors.Is(err, platform.ErrSecretNotFound) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ERROR_NOT_FOUND = %v", err)
	}
	if err := secretError("SetSecret", errorNoSuchLogonSession); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("ERROR_NO_SUCH_LOGON_SESSION = %v", err)
	}
	other := syscall.Errno(5) // ERROR_ACCESS_DENIED
	if err := secretError("SetSecret", other); !errors.Is(err, other) || errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("another error = %v", err)
	}
}
