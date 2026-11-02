package cocoa

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestSecretStatusError(t *testing.T) {
	if err := secretStatusError("Secret", errSecSuccess); err != nil {
		t.Errorf("errSecSuccess = %v, want nil", err)
	}
	if err := secretStatusError("Secret", errSecItemNotFound); !errors.Is(err, platform.ErrSecretNotFound) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errSecItemNotFound = %v, want ErrSecretNotFound", err)
	}
	for _, s := range []int32{errSecUserCanceled, errSecAuthFailed} {
		if err := secretStatusError("Secret", s); !errors.Is(err, platform.ErrSecretCanceled) {
			t.Errorf("OSStatus %d = %v, want ErrSecretCanceled", s, err)
		}
	}
	for _, s := range []int32{errSecNotAvailable, errSecNoSuchKeychain, errSecNoDefaultKeychain,
		errSecInteractionNotAllowed, errSecMissingEntitlement, errSecUnimplemented} {
		if err := secretStatusError("Secret", s); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("OSStatus %d = %v, want ErrUnsupported", s, err)
		}
	}
	err := secretStatusError(`SetSecret("a", "b")`, errSecDuplicateItem)
	if err == nil || errors.Is(err, platform.ErrUnsupported) || errors.Is(err, platform.ErrSecretCanceled) ||
		!strings.Contains(err.Error(), "-25299") || !strings.Contains(err.Error(), "SetSecret") {
		t.Errorf("another OSStatus = %v, want a plain error naming the call and the code", err)
	}
}

func TestTrashError(t *testing.T) {
	for _, c := range []struct {
		domain string
		code   int
		want   error
	}{
		{cocoaErrorDomain, nsFeatureUnsupportedError, platform.ErrUnsupported},
		{cocoaErrorDomain, nsFileWriteVolumeReadOnlyError, platform.ErrUnsupported},
		{posixErrorDomain, darwinEXDEV, platform.ErrUnsupported},
		{posixErrorDomain, darwinEROFS, platform.ErrUnsupported},
		{posixErrorDomain, darwinENOTSUP, platform.ErrUnsupported},
		{posixErrorDomain, darwinEOPNOTSUPP, platform.ErrUnsupported},
		{cocoaErrorDomain, nsFileNoSuchFileError, fs.ErrNotExist},
		{cocoaErrorDomain, nsFileReadNoSuchFileError, fs.ErrNotExist},
		{posixErrorDomain, darwinENOENT, fs.ErrNotExist},
		{cocoaErrorDomain, nsFileReadNoPermissionError, fs.ErrPermission},
		{cocoaErrorDomain, nsFileWriteNoPermissionError, fs.ErrPermission},
		{posixErrorDomain, darwinEPERM, fs.ErrPermission},
		{posixErrorDomain, darwinEACCES, fs.ErrPermission},
	} {
		err := trashError("/Users/a/x", c.domain, c.code, "described")
		if !errors.Is(err, c.want) {
			t.Errorf("%s %d = %v, want %v", c.domain, c.code, err, c.want)
		}
		if !strings.Contains(err.Error(), "described") || !strings.Contains(err.Error(), "/Users/a/x") {
			t.Errorf("%s %d = %v, want the path and the description", c.domain, c.code, err)
		}
	}
	err := trashError("/x", cocoaErrorDomain, 512, "")
	if errors.Is(err, platform.ErrUnsupported) || errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "512") {
		t.Errorf("another error = %v, want a plain error with the code", err)
	}
}

func TestBundled(t *testing.T) {
	for _, c := range []struct {
		path, id string
		want     bool
	}{
		{"/Applications/Notes.app", "dev.telaui.notes", true},
		{"/Users/a/Library/Caches/tela/Notes.APP/", "dev.telaui.notes", true},
		{"/Applications/Notes.app", "", false},
		{"/private/var/folders/x/T/go-build1/b001", "", false},
		{"/usr/local/bin", "dev.telaui.notes", false},
		{"", "dev.telaui.notes", false},
	} {
		if got := bundled(c.path, c.id); got != c.want {
			t.Errorf("bundled(%q, %q) = %v, want %v", c.path, c.id, got, c.want)
		}
	}
	for _, err := range []error{errNoBundle, errNotificationsDenied} {
		if !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("%v does not wrap ErrUnsupported", err)
		}
	}
	if !strings.Contains(errNoBundle.Error(), "tela run") {
		t.Errorf("errNoBundle = %q, want it to name tela run", errNoBundle)
	}
}

func TestNotificationNames(t *testing.T) {
	const prefix = "dev.telaui.42.1700000000."
	ident := noteIdentifier(prefix, 7)
	if id, ok := noteID(prefix, ident); !ok || id != 7 {
		t.Errorf("noteID(%q) = %d, %v; want 7", ident, id, ok)
	}
	for _, other := range []string{"dev.telaui.41.1700000000.7", prefix + "0", prefix + "x", prefix, "com.example.7"} {
		if id, ok := noteID(prefix, other); ok {
			t.Errorf("noteID(%q) = %d, want not ours", other, id)
		}
	}

	const def, dismiss = "com.apple.UNNotificationDefaultActionIdentifier", "com.apple.UNNotificationDismissActionIdentifier"
	if e := noteEvent(3, def, def, dismiss); e != (platform.NotificationEvent{ID: 3, Kind: platform.NotificationActivated}) {
		t.Errorf("default action = %+v", e)
	}
	if e := noteEvent(3, dismiss, def, dismiss); e != (platform.NotificationEvent{ID: 3, Kind: platform.NotificationClosed}) {
		t.Errorf("dismiss action = %+v", e)
	}
	if e := noteEvent(3, "reply", def, dismiss); e != (platform.NotificationEvent{ID: 3, Kind: platform.NotificationActionInvoked, Action: "reply"}) {
		t.Errorf("an app action = %+v", e)
	}

	a := []platform.NotificationAction{{ID: "open", Label: "Open"}, {ID: "later", Label: "Later"}}
	b := []platform.NotificationAction{{ID: "open", Label: "Open"}, {ID: "later", Label: "Not now"}}
	if noteCategory(nil) == noteCategory(a) || noteCategory(a) == noteCategory(b) {
		t.Errorf("categories %q, %q, %q: want one per list of actions", noteCategory(nil), noteCategory(a), noteCategory(b))
	}
	if noteCategory(a) != noteCategory(append([]platform.NotificationAction(nil), a...)) {
		t.Errorf("the same actions got two categories")
	}
}

func TestTraySizes(t *testing.T) {
	for _, c := range []struct{ thickness, want float64 }{{22, 18}, {24, 20}, {37, 33}, {0, 18}, {-1, 18}, {12, 16}} {
		if got := trayIconPoints(c.thickness); got != c.want {
			t.Errorf("trayIconPoints(%v) = %v, want %v", c.thickness, got, c.want)
		}
	}
	if w, h := trayImageSize(64, 32, 18); w != 36 || h != 18 {
		t.Errorf("trayImageSize(64x32) = %vx%v, want 36x18", w, h)
	}
	if w, h := trayImageSize(32, 32, 18); w != 18 || h != 18 {
		t.Errorf("trayImageSize(32x32) = %vx%v, want 18x18", w, h)
	}
}
