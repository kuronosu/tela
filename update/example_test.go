package update_test

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/update"
)

// updates is where the app's releases are, and the key that signs them
// (printed by tela update keygen).
var updates = update.Config{
	URL:        "https://example.com/notes/manifest.json",
	PublicKeys: []update.PublicKey{"ed25519:Wp9H1N3dD6c0m7v7kxq8nBzWJm1eJ3c0c8wzJmq7oJ0="},
}

// An app checks at start and offers a restart once the update is in
// place; UseUpdate also ends the trial of a new version (MarkHealthy).
func Example() {
	tela.Main(tela.App{
		AppID: "com.example.notes",
		Title: "Notes",
		Root:  tela.C(Root),
	})
}

func Root() tela.Node {
	up := update.UseUpdate(updates)
	return Div(Class("flex flex-col h-full"),
		If(up.Status == update.Ready && up.Update.Policy == update.ActivateByApp,
			Div(Class("flex items-center gap-3 px-4 py-2 bg-sky-100 text-sky-900"),
				Textf("Notes %s is ready.", up.Update.Version),
				Button(Class("ml-auto font-medium"), OnClick(up.Restart), Text("Restart now")),
			),
		),
		If(errors.Is(up.Err, update.ErrEndOfSupport),
			Div(Class("px-4 py-2 bg-amber-100"), Text(endOfSupport(up.Err))),
		),
		Div(Class("flex-1 p-6"), Text("Your notes")),
	)
}

func endOfSupport(err error) string {
	var eos *update.EndOfSupportError
	if errors.As(err, &eos) {
		return eos.Message + " " + eos.URL
	}
	return ""
}

// Without the hook: check, download, apply, and restart when the release
// asks for it.
func ExampleCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	u, err := update.Check(ctx, updates)
	switch {
	case errors.Is(err, update.ErrUpToDate), errors.Is(err, update.ErrManaged), errors.Is(err, update.ErrNotInstalled):
		return
	case err != nil:
		log.Print(err)
		return
	}
	log.Printf("downloading %s (%d bytes): %s", u.Version, u.Artifact.Size, u.Notes)
	if err := u.Download(ctx, func(done, total int64) { log.Printf("%d%%", done*100/total) }); err != nil {
		log.Print(err)
		return
	}
	if err := u.Apply(); err != nil {
		log.Print(err)
		return
	}
	if u.Policy == update.ActivateNow {
		_ = update.Restart()
	}
}
