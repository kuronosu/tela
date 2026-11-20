package devtools

import "github.com/kuronosu/tela"

// App is the window `tela snapshot` renders the previews in.
var App = tela.App{
	Title:     "Tela DevTools",
	Width:     1280,
	Height:    800,
	RootClass: "overflow-hidden bg-background text-foreground",
	Theme:     themePtr(),
}

func themePtr() *tela.Theme { t := Theme(); return &t }

// previewScreen is the DevTools over the made-up session with Load loaded;
// the model is made when it mounts, so nothing is built at program start.
type previewScreen struct {
	Tab  string
	Load Preload
}

// Build returns the DevTools.
func (p previewScreen) Build() tela.Node {
	m := tela.UseOnce(func() *Model {
		m := NewModel(NewFakeSource())
		m.Preload(p.Load)
		return m
	})
	return Root{Model: m, Tab: p.Tab}
}

func preview(tab string, p Preload) tela.Component { return previewScreen{tab, p} }

// Previews are the screens of `tela snapshot ./cmd/tela/internal/devtools
// --preview NAME`.
var Previews = tela.Previews{
	"Elements":      {Root: preview(TabElements, Preload{Select: FakeSettings})},
	"ElementsBox":   {Root: preview(TabElements, Preload{Select: FakeSaveBtn + 1})},
	"Accessibility": {Root: preview(TabA11y, Preload{Select: FakeSaveBtn + 1})},
	"Profiler":      {Root: preview(TabProfiler, Preload{Frames: true, Frame: 137})},
	"Logs":          {Root: preview(TabLogs, Preload{})},
}
