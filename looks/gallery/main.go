// Command gallery shows kit's controls in each of the platform looks of
// package looks: Windows 11 (Fluent), GNOME (libadwaita) and KDE (Breeze),
// each in light and dark, and switches between them while it runs.
//
//	go run ./gallery                       # the look of this OS, light or dark as the system says
//	go run ./gallery -look=breeze -dark=true
//
// One component tree draws all six looks. Each look is a tela.Theme (the
// tokens), a kit.Style (the recipes of the controls) and the Regions, the
// classes of the parts of the window that kit has no component for: the
// toolbar, the sidebar, the content, the details pane and the status bar.
// Choosing a look in the sidebar (or the arrows of the toolbar, or the menu)
// calls tela.SetTheme and gives kit.StyleProvider a new style; nothing in the
// tree asks which platform it draws.
//
// `tela snapshot` renders in one shot and takes the theme of var App, which
// starts as the look of this OS or the one the environment variable
// LOOKS_LOOK names (win11, adwaita or breeze), so name it to see another:
//
//	LOOKS_LOOK=breeze tela snapshot --size 1100x720 --dark -o gallery.png ./gallery
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kuronosu/tela"
)

// The size of the window, and of the screens the goldens show.
const windowW, windowH = 1100, 720

// parseLook resolves a look name; "" and "auto" are the look of this OS.
func parseLook(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "auto" {
		return detected().ID, nil
	}
	if c := byID(s); c != nil {
		return c.ID, nil
	}
	return "", fmt.Errorf("unknown look %q; use auto, %s", s, strings.Join(ids(), ", "))
}

// parseMode resolves the -dark flag: auto follows the system.
func parseMode(s string) (tela.ColorScheme, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return tela.System, nil
	case "true", "1", "yes", "dark":
		return tela.Dark, nil
	case "false", "0", "no", "light":
		return tela.Light, nil
	}
	return tela.System, fmt.Errorf("invalid -dark=%q; use auto, true or false", s)
}

// startLook is the look the app starts with when no flag says otherwise,
// and the one `tela snapshot` and `tela inspect` draw. They render a preview
// in one shot, which does not run the effect that swaps the theme, so the
// tokens are App.Theme's: name the look with LOOKS_LOOK to see another.
func startLook() *catalogEntry {
	id, err := parseLook(os.Getenv("LOOKS_LOOK"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gallery:", err)
		os.Exit(2)
	}
	return byID(id)
}

// defaultID is the key of the look a Gallery with no Look draws: startLook.
var defaultID = startLook().ID

// App is the window; main changes its look and scheme with the flags.
var App = func() tela.App {
	c := startLook()
	l := c.Look()
	return tela.App{
		Title:  "Gallery",
		AppID:  "dev.telaui.looks.gallery",
		Width:  windowW,
		Height: windowH,
		Theme:  &l.Theme,
		Root:   Gallery{Look: c.ID},
	}
}()

func main() {
	lookFlag := flag.String("look", "auto", "look to draw: auto (the look of this OS), "+strings.Join(ids(), ", "))
	darkFlag := flag.String("dark", "auto", "color scheme: auto (follow the system), true or false")
	flag.Parse()

	id, err := parseLook(*lookFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gallery:", err)
		os.Exit(2)
	}
	scheme, err := parseMode(*darkFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gallery:", err)
		os.Exit(2)
	}

	app := App
	l := byID(id).Look()
	app.Theme = &l.Theme
	app.ColorScheme = scheme
	app.Root = Gallery{Look: id, Scheme: scheme}
	tela.Main(app)
}

// Previews are the screens `tela snapshot --preview` can render: every look
// in both schemes. The tokens are App.Theme's (see startLook).
var Previews = func() tela.Previews {
	// The first by name is the look of App (LOOKS_LOOK or this OS) in the
	// scheme the tool says (--dark).
	p := tela.Previews{"0 Default": {Root: Gallery{}}}
	n := 1
	for _, c := range catalog {
		for _, scheme := range []tela.ColorScheme{tela.Light, tela.Dark} {
			name := "light"
			if scheme == tela.Dark {
				name = "dark"
			}
			p[fmt.Sprintf("%d %s %s", n, c.Title, name)] = tela.Preview{Root: Gallery{Look: c.ID, Scheme: scheme}}
			n++
		}
	}
	return p
}()
