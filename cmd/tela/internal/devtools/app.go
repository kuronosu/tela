package devtools

import (
	"sync"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/kit"
)

// Tabs of the DevTools.
const (
	TabElements = "elements"
	TabA11y     = "a11y"
	TabProfiler = "profiler"
	TabLogs     = "logs"
)

// endGrace is how long the window stays after the session ended, to show
// why it closes.
const endGrace = 1500 * time.Millisecond

// Run opens the DevTools window on src and returns when it closes, or when
// the session ends. Call it on the main goroutine, like tela.Main.
func Run(src Source) { tela.Main(app(src)) }

// app is the DevTools' window over src. It makes the process's windows
// dark when the system states no color scheme preference, as Chrome's and
// Flutter's DevTools are (ADR-304; the toolbar's toggle switches), so call
// it only in the process of the DevTools.
func app(src Source) tela.App {
	core.SetNoPreferenceScheme(core.ColorSchemeDark)
	theme := Theme()
	return tela.App{
		Title:     "Tela DevTools",
		AppID:     "dev.telaui.devtools",
		Width:     1280,
		Height:    800,
		MinWidth:  760,
		MinHeight: 440,
		RootClass: "overflow-hidden bg-background text-foreground",
		Theme:     &theme,
		Root:      Root{Source: src},
	}
}

var (
	styleOnce sync.Once
	style     kit.Style
)

// dtStyle returns the recipes of the DevTools, made once.
func dtStyle() *kit.Style {
	styleOnce.Do(func() { style = KitStyle() })
	return &style
}

// Root is the DevTools: a toolbar, the tabs and the panel of the selected
// one. Tab is the panel shown first ("elements" when empty).
type Root struct {
	Source Source
	// Model, when set, is used instead of a new one over Source (previews
	// give one filled with Preload).
	Model *Model
	Tab   string
}

// panelProps are what every panel receives: the view, the model to act on
// and the way to jump to an element in the Elements panel.
type panelProps struct {
	V    View
	M    *Model
	Open func(core.ElementID)
	// Details is the width of the details pane (the Elements and the
	// Accessibility panels share it), SetDetails what its splitter sets.
	Details    float32
	SetDetails func(float32)
}

// Build returns the DevTools.
func (r Root) Build() tela.Node {
	m := tela.UseOnce(func() *Model {
		if r.Model != nil {
			return r.Model
		}
		return NewModel(r.Source)
	})
	_, bump := tela.UseState(0)
	tab, setTab := tela.UseState(or(r.Tab, TabElements))
	tela.UseEffect(func() func() {
		m.OnChange(func() { bump.Update(func(n int) int { return n + 1 }) })
		m.Start()
		return func() { m.Close() }
	}, tela.Once)
	tela.UseEffectDo(func() { m.SetTab(tab) }, tab)
	v := m.View()
	// The session ended: say so for a moment, then close.
	var wait time.Duration
	if v.Conn == Ended {
		wait = endGrace
	}
	tela.UseTimeout(wait, tela.Quit, v.Conn)

	details, setDetails := tela.UseState(float32(detailsDefault))
	p := panelProps{V: v, M: m, Open: func(id core.ElementID) {
		m.Select(id, true)
		setTab.Set(TabElements)
	}, Details: details, SetDetails: setDetails.Set}
	return kit.StyleProvider{Style: dtStyle(), Children: tela.C(func() tela.Node {
		return Div(Class("flex h-full min-h-0 flex-col"),
			Toolbar{V: v, M: m},
			kit.Tabs[string]{
				Value: &tab, OnValueChange: setTab.Set, Variant: kit.TabsLine, AriaLabel: "Panels",
				Class: "min-h-0 flex-1 flex-col", ContentClass: "flex min-h-0 flex-col",
				Tabs: []kit.Tab[string]{
					{Value: TabElements, Label: "Elements", Content: ElementsPanel{panelProps: p}},
					{Value: TabA11y, Label: "Accessibility", Content: A11yPanel{panelProps: p}},
					{Value: TabProfiler, Label: "Profiler", Content: ProfilerPanel{panelProps: p}},
					{Value: TabLogs, Label: "Logs", Content: LogsPanel{panelProps: p}},
				},
			},
			StatusBar{V: v},
		)
	})}.Build()
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
