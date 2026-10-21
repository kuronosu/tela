package tela

import (
	"sort"

	"github.com/kuronosu/tela/internal/core"
)

// Preview is one screen of a package for `tela snapshot`, `tela inspect` and
// [Previews]: the component to show, and optionally a wrapper that
// supplies what the screen reads from context.
//
// Root is a [Component]: a node such as tela.C(Settings), or a struct
// component value. The local state of a preview is seeded through the fields
// of a struct component, with no global:
//
//	type Main struct{ InitialTab string }
//
//	func (m Main) Build() tela.Node {
//	    tab, setTab := tela.UseState(m.InitialTab)
//	    ...
//	}
//
//	var Previews = tela.Previews{
//	    "open": {Root: Main{InitialTab: "open"}},
//	}
//
// State that comes from a context is seeded with Wrap, which receives the
// root as a node and returns it inside the providers the screen needs:
//
//	var Previews = tela.Previews{
//	    "mocked": {
//	        Root: Main{},
//	        Wrap: func(n tela.Node) tela.Node { return AppState.Provider(mockState, n) },
//	    },
//	}
//
// A Preview is itself a Component that builds Wrap(Root): the CLI mounts
// exactly that, and a test mounts a preview the way the CLI does with
// telatest.Mount(t, Previews["mocked"]).
type Preview struct {
	// Root is the screen. It must not be nil.
	Root Component
	// Wrap, when not nil, receives the node of Root and returns what is
	// mounted instead, normally Root inside the Provider of every context
	// it reads. It runs each time the preview builds and may not use
	// hooks: put state in a component.
	Wrap func(Node) Node
}

// Build returns the node the preview mounts: Root, or Wrap(Root) when Wrap
// is set. It panics when Root is nil.
func (p Preview) Build() Node {
	if p.Root == nil {
		panic("tela: Preview needs a Root; for example tela.Preview{Root: tela.C(Settings)} or tela.Preview{Root: Settings{}} for a struct component")
	}
	n := core.NodeOf(p.Root)
	if p.Wrap != nil {
		return p.Wrap(n)
	}
	return n
}

// Previews are the named screens of a package, for `tela snapshot` and
// `tela inspect` (ARCHITECTURE section 17). Define one as a package-level
// variable called Previews:
//
//	var Previews = tela.Previews{
//	    "Login":    {Root: tela.C(LoginScreen)},
//	    "Settings": {Root: Settings{InitialTab: "account"}},
//	}
//
// Each Root is a component, so it may use hooks. The CLI renders the
// preview selected with --preview NAME; without the flag it renders the
// first name in byte order (see [Previews.Names]), so a name such as "1
// Login" can pick the default. A package with a single screen can define
// `func Preview() tela.Node` (or tela.Component) instead. See [Preview] to
// start a screen in a given state.
type Previews map[string]Preview

// Names returns the preview names in the order the CLI uses: sorted by
// bytes. The first is the default preview.
func (p Previews) Names() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
