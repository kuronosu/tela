package main

import (
	"sync"

	"github.com/kuronosu/tela"
)

// closeGate connects App.OnClose to the component tree. App.OnClose is a
// plain function of the App value, outside the tree, but what closing the
// window does depends on state: the "keep running in the tray" setting, the
// notes with unsaved changes. The screen registers its latest handler here
// after every build, and takes it back when it unmounts.
type closeGate struct {
	mu   sync.Mutex
	fn   func(tela.WindowHandle)
	quit func()
}

// set registers the handlers of the current build: close for App.OnClose
// and quit for App.OnQuit (nil removes them).
func (g *closeGate) set(fn func(tela.WindowHandle), quit func()) {
	g.mu.Lock()
	g.fn, g.quit = fn, quit
	g.mu.Unlock()
}

// closeMain is App.OnClose. Without a screen to ask (the app is starting or
// ending) closing the window quits.
func (g *closeGate) closeMain(w tela.WindowHandle) {
	g.mu.Lock()
	fn := g.fn
	g.mu.Unlock()
	if fn == nil {
		tela.Quit()
		return
	}
	fn(w)
}

// quitApp is App.OnQuit: Cmd+Q, the app menu's Quit, the Dock's Quit and a
// logout on macOS. It really quits, whatever the "keep running in the tray"
// setting says, after the same question about unsaved notes that closing the
// window asks when that setting is off. Without a screen to ask it just quits.
func (g *closeGate) quitApp() {
	g.mu.Lock()
	fn := g.quit
	g.mu.Unlock()
	if fn == nil {
		tela.Quit()
		return
	}
	fn()
}

// gate is the app's close gate.
var gate closeGate
