// Package aqua is the look of macOS (ADR-197, ADR-223), in two designs that an
// app can mix part by part:
//
//   - [Tahoe], the "Liquid Glass" of macOS 26: capsule controls (buttons,
//     fields, selects and segmented controls are pills), large radii, a
//     toolbar whose groups are floating glass and a sidebar that is an inset
//     glass panel, over an opaque content area. The accent is 0088FF
//     (0A91FF in dark).
//   - [Sequoia], the classic Aqua of macOS 11 to 15: rounded rectangles of
//     6 px, a flat toolbar unified with the title bar, a plain full height
//     sidebar, white push buttons with a hairline shadow, small 14 px
//     checkboxes and radios, 38x22 switches. The accent is 007AFF (0A84FF in
//     dark).
//
// [Look] with no option is Tahoe. The options set the parts of the look, and
// apply in order, so the last one that sets a part wins:
//
//	aqua.Look()                                  // Tahoe
//	aqua.Look(aqua.Sequoia)                      // Sequoia
//	aqua.Look(aqua.Sequoia, aqua.Chrome(aqua.Tahoe)) // Sequoia's controls and tokens, Tahoe's window
//	aqua.Look(aqua.Tahoe, aqua.GlassContent())   // Tahoe with glass cards in the content
//
// A design is an [Option] that sets every part (it also clears
// [GlassContent]). [Controls] sets the controls: buttons, fields, toggles,
// checkboxes, radios, switches, selects, sliders and segmented controls (and
// the progress bar and the list a select opens). [Chrome] sets the frame of
// the window: the [looks.Regions] (the toolbar, its groups, its location bar
// and search field, the sidebar and its entries, the content, the details pane
// and the status bar) and the sidebar and toolbar variants of kit's Toggle.
// [GlassContent] makes the cards of the content glass (the real Tahoe keeps
// its content opaque, so it is an option of its own). What no option names
// (the tokens, the radius, the tables, menus, dialogs and tooltips) belongs
// to the design given to [Look] last: every combination has the complete
// tokens of both designs for light and dark, so any mix is a consistent look.
// The name of a mix says so: "macOS Tahoe (mixed: Sequoia controls)".
//
// # What Tela cannot reproduce
//
// Glass is a backdrop blur, a translucent fill, a light 1 px edge and an inset
// highlight along the top. Tela does not refract: the light bending at the
// edges of Apple's glass, its lensing and its adaptive tint are not drawn.
// The font is Inter and the icons Lucide's (the app's), never Apple's San
// Francisco and SF Symbols, and no other asset of Apple's is used.
//
// Glass costs a backdrop blur on the CPU for each piece, on every frame in
// which what is behind it changes; the blur is cached while its input does not
// change (ADR-200). [Sequoia] has no blur at all.
//
// The tokens beyond kit's are the ones of package looks, and Tahoe's glass
// ones (glass, glass-strong, glass-selected, glass-edge, glass-highlight,
// glass-shadow and primary-glow) and Sequoia's (control-shadow, segment),
// which both designs define.
package aqua

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/internal/base"
)

// Design is one of the two designs of macOS: [Tahoe] and [Sequoia]. As an
// [Option] it sets every part of the look; given to [Controls] or [Chrome] it
// names the design of that part.
type Design int

const (
	// Tahoe is the design of macOS 26: capsules and glass.
	Tahoe Design = iota
	// Sequoia is the design of macOS 11 to 15: rounded rectangles and flat
	// surfaces.
	Sequoia
)

func (d Design) String() string {
	switch d {
	case Tahoe:
		return "Tahoe"
	case Sequoia:
		return "Sequoia"
	}
	return fmt.Sprintf("Design(%d)", int(d))
}

// valid panics unless d is one of the designs.
func (d Design) valid(what string) {
	if d != Tahoe && d != Sequoia {
		panic(fmt.Sprintf("aqua: %s is not a design (%d); use aqua.Tahoe or aqua.Sequoia", what, int(d)))
	}
}

// Option changes a part of the look that [Look] returns. The designs, [Controls],
// [Chrome] and [GlassContent] are options.
type Option interface {
	apply(*config)
}

// config is what the options build: the design of each part.
type config struct {
	base     Design // the tokens, the radius, tables, menus, dialogs, tooltips and cards
	controls Design
	chrome   Design
	glass    bool // glass cards in the content
}

// apply makes d the design of every part. It is what makes a Design an
// Option.
func (d Design) apply(c *config) {
	d.valid("the option")
	*c = config{base: d, controls: d, chrome: d}
}

type controlsOption Design

func (o controlsOption) apply(c *config) { c.controls = Design(o) }

type chromeOption Design

func (o chromeOption) apply(c *config) { c.chrome = Design(o) }

type glassOption struct{}

func (glassOption) apply(c *config) { c.glass = true }

// Controls sets the design of the controls: buttons, fields, toggles,
// checkboxes, radios, switches, selects, sliders and segmented controls (and
// the progress bar), and the search field of the toolbar, which is an input
// ([looks.Regions].SearchField and SearchIcon follow Controls, not [Chrome]).
// The tokens stay those of the design given to [Look].
func Controls(d Design) Option {
	d.valid("the argument of Controls")
	return controlsOption(d)
}

// Chrome sets the design of the window's frame: the toolbar with its groups,
// location bar and the room of the search field, the sidebar with its
// entries, the content, the details pane and the status bar. The tokens stay
// those of the design given to [Look].
func Chrome(d Design) Option {
	d.valid("the argument of Chrome")
	return chromeOption(d)
}

// GlassContent makes the cards of the content ([kit.Card]) glass: a blurred,
// translucent fill with Tahoe's edge and highlight, which shows what is behind
// the card. Tahoe's content is opaque, so with no option the cards are opaque
// too. Setting a design after it clears it.
func GlassContent() Option { return glassOption{} }

// Look returns the look of macOS: [Tahoe], or what the options make. Each call
// returns a new one: its Style may be edited. An option that is not a design
// from this package panics.
func Look(opts ...Option) looks.Look {
	c := config{base: Tahoe, controls: Tahoe, chrome: Tahoe}
	for _, o := range opts {
		if o == nil {
			panic("aqua: Look was given a nil option")
		}
		o.apply(&c)
	}

	s := base.Style(fontSize)
	surfaces(&s, c.base)
	controls(&s, c.controls)
	frame(&s, c.chrome)
	if c.glass {
		glassCards(&s)
	}
	return looks.Look{
		Name:    c.name(),
		Theme:   theme(c.base),
		Style:   &s,
		Regions: c.regions(),
	}
}

// fontSize is the text size of both designs.
const fontSize = "text-[13px]"

// name says which design the look is, and "mixed" with the parts that differ
// from the base when they do.
func (c config) name() string {
	var parts []string
	switch {
	case c.controls != c.base && c.controls == c.chrome:
		parts = append(parts, c.controls.String()+" controls and chrome")
	case c.controls != c.base:
		parts = append(parts, c.controls.String()+" controls")
	case c.chrome != c.base:
		parts = append(parts, c.chrome.String()+" chrome")
	}
	if c.glass {
		parts = append(parts, "glass content")
	}
	n := "macOS " + c.base.String()
	if len(parts) == 0 {
		return n
	}
	return n + " (mixed: " + strings.Join(parts, ", ") + ")"
}

// theme is the tokens of a design.
func theme(d Design) tela.Theme {
	if d == Tahoe {
		return tahoeTheme()
	}
	return sequoiaTheme()
}

// surfaces sets what no option names: tables, menus, dialogs, tooltips and
// cards.
func surfaces(s *kit.Style, d Design) {
	if d == Tahoe {
		tahoeSurfaces(s)
	} else {
		sequoiaSurfaces(s)
	}
}

// controls sets the controls of a design.
func controls(s *kit.Style, d Design) {
	if d == Tahoe {
		tahoeControls(s)
	} else {
		sequoiaControls(s)
	}
}

// frame sets the toggle variants of the sidebar and the toolbar.
func frame(s *kit.Style, d Design) {
	if d == Tahoe {
		tahoeFrame(s)
	} else {
		sequoiaFrame(s)
	}
}

// regions is the classes of the window: the chrome's, except for the search
// field, which is an input and so follows the controls: Sequoia's is a
// borderless gray field with its magnifier close to the edge, Tahoe's the glass
// capsule of the input recipe (nothing to add).
func (c config) regions() looks.Regions {
	var r looks.Regions
	if c.chrome == Tahoe {
		r = tahoeRegions()
	} else {
		r = sequoiaRegions()
	}
	if c.controls == Tahoe {
		r.SearchIcon, r.SearchField = tahoeSearchIcon, ""
	} else {
		r.SearchIcon, r.SearchField = sequoiaSearchIcon, sequoiaSearchField
	}
	return r
}
