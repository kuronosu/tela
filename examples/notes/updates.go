package main

import (
	"errors"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/update"
)

// Updates (package update, ADR-296). Where Notes's updates are and the key
// that signs them are given when the program is packaged, with the linker's
// -X flag on these variables:
//
//	tela package -os linux,windows,darwin -arch amd64 -version 1.1.0 \
//	    -ldflags "-X main.updateURL=https://example.com/notes/manifest.json -X main.updateKey=ed25519:..." \
//	    ./examples/notes
//
// Without them (go run, tela dev, the tests) Notes does not check for
// updates. With them it checks once at start, on a goroutine of its own
// (update.UseUpdate), downloads the update for the way it was installed and
// puts it in place, and the rail shows a small notice with a Restart button
// once it is ready; nothing while it checks, downloads or is up to date.
// A deb, flatpak or MSIX install is updated by its package manager, and a
// program that is not an install (go build) has nothing to update: both
// say nothing either.
var (
	// updateURL is the URL of the update manifest (manifest.json; its
	// signature is manifest.json.sig next to it).
	updateURL string
	// updateKey is the public key that signs the manifest, as tela update
	// keygen prints it ("ed25519:...").
	updateKey string
)

// updatesOn reports whether this build checks for updates.
func updatesOn() bool { return updateURL != "" && updateKey != "" }

// updateConfig is the Config of this build.
func updateConfig() update.Config {
	return update.Config{URL: updateURL, PublicKeys: []update.PublicKey{update.PublicKey(updateKey)}}
}

// useUpdate and restart are package update's; the tests replace them.
var (
	useUpdate = update.UseUpdate
	restart   = update.Restart
)

// updateCheck checks for an update while it is mounted (once: the screen
// mounts it only when updatesOn), and shows updateReady when one is ready.
// A failure that is news to nobody (a managed install, a program that is
// not an install, no network) is written to the log, not shown.
type updateCheck struct{}

func (updateCheck) Build() tela.Node {
	up := useUpdate(updateConfig())
	tela.UseEffect(func() func() {
		if up.Status == update.Failed && !errors.Is(up.Err, update.ErrManaged) && !errors.Is(up.Err, update.ErrNotInstalled) {
			desktop.Logger().Warn("update check failed", "err", up.Err)
		}
		return nil
	}, up.Status)
	if up.Status != update.Ready || up.Update == nil {
		return nil
	}
	return updateReady{Version: up.Update.Version, OnRestart: func() {
		if err := restart(); err != nil {
			desktop.Logger().Warn("restart after the update failed", "err", err)
		}
	}}
}

// updateReady is the notice of an update that is in place: the version and
// a Restart button, which starts it now (else it runs from the next start).
// It is a quiet card at the foot of the rail, above Preferences, and a
// polite live region, so a screen reader says it once without interrupting.
type updateReady struct {
	Version   string
	OnRestart func()
}

func (u updateReady) Build() tela.Node {
	return kit.Alert{
		Role:             "status",
		Icon:             icons.Download,
		Class:            "gap-x-2 rounded-lg border-border bg-card/70 px-2.5 py-2 text-[12px]",
		Title:            Textf("Notes %s is ready", u.Version),
		TitleClass:       "text-[12px] line-clamp-2",
		DescriptionClass: "text-[12px] gap-2",
		Description: Fragment(
			Text("Restart to use it."),
			kit.Button{Variant: kit.Outline, Size: kit.Xs, OnClick: u.OnRestart, Children: Text("Restart")},
		),
	}
}
