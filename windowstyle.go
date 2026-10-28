package tela

import "github.com/kuronosu/tela/internal/core"

// TitleBarColors are the colors of the title bar and border the OS draws
// for a window ([App].TitleBar, [Window].TitleBar). A zero Color, or one
// with zero alpha, keeps the system's color; the alpha of any other is
// ignored (the color is used opaque).
//
//	tela.App{
//	    TitleBar: tela.TitleBarColors{
//	        Background: tela.Hex("#1e1b4b"),
//	        Text:       tela.Hex("#e0e7ff"),
//	    },
//	    ...
//	}
//
// Windows 11 (build 22000 and later) applies them to the native title bar
// of a decorated window (Background, Text: the title and the glyphs of
// the caption buttons) and to its border (Border, also the thin border
// Windows 11 draws around a Frameless window). Windows 10, Linux (Wayland
// and X11) and headless ignore them, as does a decorated window on
// Wayland whose compositor draws no title bar for it. The dark or light
// title bar already follows the app's color scheme ([App].ColorScheme);
// these colors are on top of it. Changing the field of an open [Window]
// updates the window.
type TitleBarColors = core.TitleBarColors

// Backdrop is the system material behind a window ([App].Backdrop,
// [Window].Backdrop): Mica, Mica Alt or Acrylic on Windows 11.
//
// The zero value, [BackdropDefault], leaves the choice to the system. On
// Windows 11 22H2 (build 22621 and later) the other values apply the
// material to the native title bar of a decorated window, always; and, for
// [BackdropMica], [BackdropMicaAlt] and [BackdropAcrylic], to the content
// of the window where the app leaves it transparent. The root has no
// background by default (see RootClass), so a root without a background
// class shows the material, a root with a translucent one tints it, and the
// surfaces the app draws over it (a sidebar with bg-white/60, a card with
// bg-card) cover it. A root with bg-background hides it:
//
//	tela.App{
//	    Backdrop:  tela.BackdropMica,
//	    RootClass: "bg-background/60", // Mica shows through; elsewhere a plain background
//	    Root:      Notes{},
//	}
//
// The material follows the dark or light title bar, which follows the
// app's color scheme. A [Frameless] window with a client-area backdrop
// extends the window frame over its content (DwmExtendFrameIntoClientArea),
// which Windows 11 may answer by no longer showing the snap layouts flyout
// over the app's maximize button; keep a frameless window without a
// backdrop if that matters.
//
// Windows 10, Linux and headless ignore Backdrop, and so does Windows 11
// before 22H2: the window stays as without it, so an app that leaves its
// root transparent should still give it a background color for them (a
// dark: class pair, or a background only where the backdrop is missing).
// Changing the field of an open [Window] updates the window.
type Backdrop = core.Backdrop

const (
	// BackdropDefault leaves the choice to the system: no material for a
	// plain window.
	BackdropDefault = core.BackdropDefault
	// BackdropNone turns the system material off.
	BackdropNone = core.BackdropNone
	// BackdropMica is Mica, the opaque material tinted with the desktop
	// wallpaper, for the main windows of an app.
	BackdropMica = core.BackdropMica
	// BackdropMicaAlt is Mica Alt, a stronger tint, for windows with tabs
	// in their title bar.
	BackdropMicaAlt = core.BackdropMicaAlt
	// BackdropAcrylic is Acrylic, the translucent blurred material of
	// transient surfaces such as flyouts.
	BackdropAcrylic = core.BackdropAcrylic
)
