// Command native is a file explorer that looks native on Windows 11
// (Fluent), GNOME (Adwaita) and KDE (Breeze), each in light and dark.
//
// There is one component tree. A Theme (theme.go and the theme_*.go files)
// supplies the class strings of every part, the column layout and the way
// dates and sizes read; nothing in the tree knows which platform it draws.
// The icons are boxes drawn with classes (icons.go), and the file system is
// an in-memory tree (fs.go).
//
//	go run ./examples/native                       # the theme of this OS, light or dark as the system says
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
	if _, ok := Themes[s]; ok {
		return s, nil
	}
	return "", fmt.Errorf("unknown theme %q; use auto, %s", s, strings.Join(ThemeIDs, ", "))
}

// parseMode resolves the -dark flag: auto follows the system.
func parseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return Auto, nil
	case "true", "1", "yes", "dark":
		return Dark, nil
	case "false", "0", "no", "light":
		return Light, nil
	}
	return Auto, fmt.Errorf("invalid -dark=%q; use auto, true or false", s)
}

// The size of the window, and of the screens the goldens show.
const windowW, windowH = 1280, 760

func main() {
	themeFlag := flag.String("theme", "auto", "look to draw: auto (win11 on Windows, adwaita elsewhere), "+strings.Join(ThemeIDs, ", "))
	darkFlag := flag.String("dark", "auto", "color scheme: auto (follow the system), true or false")
	flag.Parse()

	id, err := parseTheme(*themeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "native:", err)
		os.Exit(2)
	}
	mode, err := parseMode(*darkFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "native:", err)
		os.Exit(2)
	}

	th := Themes[id]
	if err := tela.Run(tela.App{
		Title:  th.Title,
		AppID:  "dev.telaui.native",
		Width:  windowW,
		Height: windowH,
		Root: func() tela.Node {
			return tela.CP(Explorer, Props{Theme: id, Mode: mode, Path: "Documents", Sel: "Contract draft.docx"})
		},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "native:", err)
		os.Exit(1)
	}
}

// Previews are the screens `tela snapshot --preview` can render: every
// theme in both schemes, on the Documents folder with a file selected.
var Previews = func() tela.Previews {
	p := tela.Previews{}
	n := 1
	for _, id := range ThemeIDs {
		for _, m := range []Mode{Light, Dark} {
			th, mode := Themes[id], m
			p[fmt.Sprintf("%d %s %s", n, th.Name(), mode)] = func() tela.Node {
				return tela.CP(Explorer, Props{Theme: th.ID, Mode: mode, Path: "Documents", Sel: "Contract draft.docx"})
			}
			n++
		}
	}
	return p
}()
