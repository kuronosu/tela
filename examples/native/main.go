// Command native is a file explorer that looks native on Windows 11
// (Fluent), GNOME (Adwaita) and KDE (Breeze), each in light and dark, built
// only on package kit.
//
// There is one component tree. Each look is three things (look.go): the
// tokens of a tela.Theme (colors in light and dark, the base radius, the
// font), a kit.Style (the recipes of the buttons, the table, the menus and
// the dialogs of that platform) and a few classes for the regions the
// explorer lays out itself. kit.StyleProvider and tela.SetTheme swap them
// at run time; nothing in the tree knows which platform it draws. The icons
// are Lucide's (icons/), and the file system is an in-memory tree (fs.go).
//
//	go run ./examples/native                       # the look of this OS, light or dark as the system says
//	go run ./examples/native -theme=breeze -dark=true
//
// `tela snapshot --preview "1 Windows 11 light" ./examples/native` renders
// one of the Previews without a window.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/kuronosu/tela"
)

// parseTheme resolves the -theme flag: "auto" is win11 on Windows and
// adwaita elsewhere.
func parseTheme(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "auto":
		if runtime.GOOS == "windows" {
			return "win11", nil
		}
		return "adwaita", nil
	}
	if _, ok := Looks[s]; ok {
		return s, nil
	}
	return "", fmt.Errorf("unknown theme %q; use auto, %s", s, strings.Join(LookIDs, ", "))
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

// The size of the window, and of the screens the goldens show.
const windowW, windowH = 1280, 760

// startLook is the look the app starts with when no flag says otherwise,
// and the one `tela snapshot` and `tela inspect` draw: the look of this OS,
// or the one named by NATIVE_LOOK. They render a preview in one shot, which
// does not run the effect that swaps the theme, so every preview takes the
// tokens of App.Theme (the look of the preview is the tree's, the colors
// are the theme's): to see another look with the tool, name it,
//
//	NATIVE_LOOK=breeze tela snapshot --preview "5 KDE light" ./examples/native
func startLook() *Look {
	id, err := parseTheme(os.Getenv("NATIVE_LOOK"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "native:", err)
		os.Exit(2)
	}
	return Looks[id]
}

// App is the window; main changes its look and scheme with the flags.
var App = func() tela.App {
	l := startLook()
	return tela.App{
		Title:  l.Title,
		AppID:  "dev.telaui.native",
		Width:  windowW,
		Height: windowH,
		Theme:  &l.Theme,
		Root:   Explorer{Look: l.ID, Path: "Documents", Sel: "Contract draft.docx"},
	}
}()

func main() {
	themeFlag := flag.String("theme", "auto", "look to draw: auto (win11 on Windows, adwaita elsewhere), "+strings.Join(LookIDs, ", "))
	darkFlag := flag.String("dark", "auto", "color scheme: auto (follow the system), true or false")
	flag.Parse()

	id, err := parseTheme(*themeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "native:", err)
		os.Exit(2)
	}
	scheme, err := parseMode(*darkFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "native:", err)
		os.Exit(2)
	}

	app := App
	l := Looks[id]
	app.Title, app.Theme, app.ColorScheme = l.Title, &l.Theme, scheme
	app.Root = Explorer{Look: id, Path: "Documents", Sel: "Contract draft.docx"}
	tela.Main(app)
}

// Previews are the screens `tela snapshot --preview` can render: every
// look in both schemes, on the Documents folder with a file selected.
var Previews = func() tela.Previews {
	p := tela.Previews{}
	n := 1
	for _, id := range LookIDs {
		for _, scheme := range []tela.ColorScheme{tela.Light, tela.Dark} {
			name := "light"
			if scheme == tela.Dark {
				name = "dark"
			}
			p[fmt.Sprintf("%d %s %s", n, Looks[id].Name, name)] = tela.Preview{
				Root: Explorer{Look: id, Scheme: scheme, Path: "Documents", Sel: "Contract draft.docx"},
			}
			n++
		}
	}
	return p
}()
