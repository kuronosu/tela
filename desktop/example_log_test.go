package desktop_test

import (
	"log/slog"
	"os"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
)

// The app's log: JSON lines in LogDir, text on the terminal when there is
// one. Name the app before the first record.
func ExampleLogger() {
	desktop.SetAppID("com.example.notes")
	desktop.SetLogLevel(slog.LevelDebug)
	log := desktop.Logger()
	log.Info("note saved", "id", 7, "bytes", 1024)
	log.Error("sync failed", "err", os.ErrDeadlineExceeded)
}

// A crash report of an earlier run reaches the app after its first frame,
// on the UI thread. It decides what to do with it: Tela sends nothing.
func ExampleOnCrash() {
	desktop.OnCrash(func(c desktop.Crash) {
		desktop.Logger().Warn("the last run crashed", "version", c.Version, "time", c.Time, "report", c.Path)
		// Offer the user to send c.Report, or keep the file for support.
	})
	tela.Main(tela.App{AppID: "com.example.notes", Root: tela.C(func() tela.Node {
		return Text("Notes")
	})})
}
