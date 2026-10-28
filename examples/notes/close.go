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
	mu sync.Mutex
	fn func(tela.WindowHandle)
}

// set registers the handler of the current build (nil removes it).
func (g *closeGate) set(fn func(tela.WindowHandle)) {
	g.mu.Lock()
	g.fn = fn
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

// gate is the app's close gate.
var gate closeGate
