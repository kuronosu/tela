package desktop

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// Window placements are kept in the store windows.json in [StateDir], one
// key per window. A placement changes many times while the user drags or
// resizes a window, so that store waits longer before it writes.
const placementDelay = 500 * time.Millisecond

// placements returns the store of the window placements.
func placements() *Store {
	dir, err := dirPath(dirState)
	if err != nil {
		return memoryStore("windows", err)
	}
	s, _ := OpenStore(filepath.Join(dir, "windows.json"))
	s.mu.Lock()
	s.delay = placementDelay
	s.mu.Unlock()
	return s
}

// LoadPlacement returns the window placement saved under key (by
// [UseSavePlacement] or [SavePlacement]), or the zero placement when none
// was saved: give it to tela.App.Placement or tela.Window.Placement, and
// the window opens where the user left it. Safe from any goroutine,
// before tela.Run too (call [SetAppID] first, or use [PersistApp]).
func LoadPlacement(key string) tela.WindowPlacement {
	var p tela.WindowPlacement
	placements().Get(key, &p)
	return p
}

// SavePlacement saves p under key ([UseSavePlacement] does it by itself).
// The zero placement deletes the saved one.
func SavePlacement(key string, p tela.WindowPlacement) error {
	return savePlacement(placements(), key, p)
}

// savePlacement is SavePlacement on a store already resolved.
func savePlacement(s *Store, key string, p tela.WindowPlacement) error {
	if p == (tela.WindowPlacement{}) {
		s.Delete(key)
		return nil
	}
	return s.Set(key, p)
}

// UseSavePlacement saves the placement of the window the calling
// component renders in (tela.UseWindow) under key: when it changes (the
// user moved, resized, maximized it; written half a second after the
// last change) and when the component unmounts (the window closes, the
// app quits). Read it back with [LoadPlacement]:
//
//	tela.Window{
//	    Title:     "Inspector",
//	    Placement: desktop.LoadPlacement("inspector"),
//	    OnClose:   func() { setOpen.Set(false) },
//	    Root:      Inspector{}, // Inspector's Build calls desktop.UseSavePlacement("inspector")
//	}
//
// [SavedWindow] and [PersistApp] do both. key must stay the same for the
// life of the component. A tree without windows (tela snapshot) saves
// nothing.
func UseSavePlacement(key string) {
	if key == "" {
		panic("desktop.UseSavePlacement(\"\"): give the window a key, such as \"main\" or \"inspector\"")
	}
	w := tela.UseWindow()
	tela.UseMount(func() func() {
		if _, ok := core.DesktopOf(w); !ok {
			return nil
		}
		// Resolved once: a drag reports a placement for every event.
		st := placements()
		save := func(p tela.WindowPlacement) {
			if p != (tela.WindowPlacement{}) {
				_ = savePlacement(st, key, p)
			}
		}
		save(w.Placement())
		stop := core.WatchPlacement(w, save)
		return func() {
			stop()
			save(w.Placement()) // kept after the window closed
		}
	})
}

// SavedWindow is a tela.Window that opens where the user left it: the
// placement saved under Key is its Placement (unless Window.Placement is
// set), and its placement is saved under Key when it changes and when it
// closes ([UseSavePlacement]).
//
//	If(inspectorOpen, tela.Component(desktop.SavedWindow{
//	    Key: "inspector",
//	    Window: tela.Window{
//	        Title: "Inspector", Width: 360, Height: 600,
//	        OnClose: func() { setInspectorOpen.Set(false) },
//	        Root:    Inspector{},
//	    },
//	}))
type SavedWindow struct {
	// Key names the window's saved placement; required and unique in the
	// app ("main" is PersistApp's).
	Key string
	// Window is the window to open.
	Window tela.Window
}

// Build opens the window (see [SavedWindow]).
func (s SavedWindow) Build() tela.Node {
	if s.Key == "" {
		panic(fmt.Sprintf("desktop.SavedWindow %q has no Key: give it the name its placement is saved under, such as Key: \"inspector\"", s.Window.Title))
	}
	saved := tela.UseOnce(func() tela.WindowPlacement { return LoadPlacement(s.Key) })
	w := s.Window
	if w.Placement == (tela.WindowPlacement{}) {
		w.Placement = saved
	}
	if w.Root != nil {
		w.Root = placementRoot{Key: s.Key, Root: w.Root}
	}
	return h.Fragment(w)
}

// placementRoot is a window's root that saves its placement.
type placementRoot struct {
	Key  string
	Root tela.Component
}

func (r placementRoot) Build() tela.Node {
	UseSavePlacement(r.Key)
	return h.Fragment(r.Root)
}

// MainWindowKey is the key [PersistApp] saves the main window's placement
// under.
const MainWindowKey = "main"

// PersistApp returns app with the main window's placement remembered: the
// placement saved under [MainWindowKey] becomes App.Placement (unless it
// is set), and the root saves it while the app runs and when it quits.
// When app.AppID is set and [SetAppID] was not called, it names the app's
// directories ([SetAppID]), so the placement is read before tela.Run from
// the same folder it is saved to.
//
//	func main() {
//	    tela.Main(desktop.PersistApp(tela.App{
//	        AppID: "com.example.notes",
//	        Title: "Notes", Width: 900, Height: 640,
//	        Root:  Notes{},
//	    }))
//	}
func PersistApp(app tela.App) tela.App {
	if app.AppID != "" {
		appID.mu.Lock()
		unset := appID.set == ""
		appID.mu.Unlock()
		if unset {
			SetAppID(app.AppID)
		}
	}
	if app.Placement == (tela.WindowPlacement{}) {
		app.Placement = LoadPlacement(MainWindowKey)
	}
	if app.Root != nil {
		app.Root = placementRoot{Key: MainWindowKey, Root: app.Root}
	}
	return app
}
