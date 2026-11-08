package telatest

import (
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/text"
)

// DefaultTimeout is how long actions and expectations wait for their
// condition when no [Timeout] is given.
const DefaultTimeout = 5 * time.Second

// Option configures [Mount] and the headless entry points [Render],
// [AriaSnapshot] and [LayoutJSON].
type Option func(*config)

type config struct {
	width, height float32
	scale         float64
	dark          bool
	reducedMotion bool
	highContrast  bool
	textScale     float64
	accent        *tela.Color
	ignoreText    bool
	scheme        tela.ColorScheme
	classic       bool
	instant       bool
	nativeMenus   bool
	dev           bool
	allowDev      []string
	rootClass     string
	timeout       time.Duration
	checkDamage   bool
	debug         []string
	debugClock    func() time.Duration
	noSettle      bool
	theme         *tela.Theme
	aliases       map[string]string
	fonts         []tela.Font
	// win is the OS window of the main window (winoptions.go), screen the
	// size of the fake screen maximized and fullscreen windows fill.
	win    windowConfig
	screen platform.Size
	// noServerDeco: NoServerDecorations.
	noServerDeco bool
	// controls: TitleBarControls; unsupported: WithUnsupported.
	controls    tela.Rect
	unsupported []DesktopService
	// flavor: PlatformKeys (zero: the host's).
	flavor platform.Flavor
}

func newConfig(opts []Option) config {
	cfg := config{scale: 1, timeout: DefaultTimeout, checkDamage: os.Getenv(EnvCheckDamage) == "1", dev: true}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return cfg
}

// validate reports a configuration that cannot work, with the fix.
func (c config) validate() error {
	switch {
	case c.width < 0 || c.height < 0 || (c.width == 0) != (c.height == 0):
		return fmt.Errorf("telatest.Size(%g, %g): give a positive width and height in logical pixels, such as telatest.Size(800, 600)", c.width, c.height)
	case !(c.scale > 0):
		return fmt.Errorf("telatest.Scale(%g): the display scale must be positive, such as telatest.Scale(2)", c.scale)
	case c.timeout <= 0:
		return fmt.Errorf("telatest.Timeout(%v): the timeout must be positive, such as telatest.Timeout(2 * time.Second)", c.timeout)
	case c.textScale != 0 && !(c.textScale > 0 && c.textScale <= platform.MaxTextScale):
		return fmt.Errorf("telatest.TextScale(%g): give a factor above 0 and at most %d, such as telatest.TextScale(1.25)", c.textScale, platform.MaxTextScale)
	}
	if _, err := core.ParseDebug(strings.Join(c.debug, ",")); err != nil {
		return fmt.Errorf("telatest.Debug: %v", err)
	}
	for _, f := range c.coreFonts() {
		// Verify reads and parses the font now, so a broken file fails
		// the test instead of drawing fallback glyphs.
		if err := f.Verify(); err != nil {
			return fmt.Errorf("telatest.Fonts: %v", err)
		}
	}
	if _, err := headlessServices(c.unsupported); err != nil {
		return fmt.Errorf("telatest.WithUnsupported%v", err)
	}
	if r := c.controls; !finiteRect(r) || r.Width < 0 || r.Height < 0 {
		return fmt.Errorf("telatest.TitleBarControls(%v): give a finite rectangle in logical pixels with a width and height that are not negative", r)
	}
	return c.win.validate(c.screen)
}

// titleBarControls returns the rectangle the fake window manager's
// buttons cover in TitleBarOverlay windows: the one the test gave, else
// macOS's under PlatformKeys("darwin").
func (c config) titleBarControls() platform.Rect {
	r := c.controls
	if r == (tela.Rect{}) && c.flavor == platform.FlavorMac {
		r = MacTitleBarControls
	}
	return platform.Rect{X: r.X, Y: r.Y, W: r.Width, H: r.Height}
}

// coreFonts returns the app's fonts for core.
func (c config) coreFonts() []core.Font {
	out := make([]core.Font, len(c.fonts))
	for i, f := range c.fonts {
		out[i] = core.Font(f)
	}
	return out
}

// fontCollection returns the collection the app draws with: its own fonts,
// then the deterministic set (the same fonts as fonts/standard, ADR-244),
// and never the system's.
func (c config) fontCollection() *text.Collection {
	// Not an error: validate has checked the fonts and there are no system
	// fonts to fail.
	coll, _ := core.NewFontCollection(c.coreFonts(), stdfonts.Sources(), nil)
	return coll
}

// coreConfig returns the tree configuration (without Log and IDPrefix).
func (c config) coreConfig() core.Config {
	// TELA_DEBUG is ignored (goldens must not depend on the environment):
	// the overlays are the ones Debug asked for, or none.
	dbg, _ := core.ParseDebug(strings.Join(c.debug, ","))
	if dbg == 0 {
		dbg = core.DebugOff
	}
	clock := c.debugClock
	if clock == nil {
		// A deterministic clock: every frame took one millisecond.
		var now time.Duration
		clock = func() time.Duration { now += time.Millisecond; return now }
	}
	cc := core.Config{
		// The app's fonts and the deterministic ones (ADR-008, ADR-244),
		// never the system's: goldens are the same on every machine.
		Fonts:             c.fontCollection(),
		Debug:             dbg,
		DebugClock:        clock,
		RootClass:         c.rootClass,
		Dark:              c.dark,
		ReducedMotion:     c.reducedMotion,
		HighContrast:      c.highContrast,
		TextScale:         c.textScale,
		IgnoreTextScale:   c.ignoreText,
		ColorScheme:       c.scheme,
		Dev:               c.devMode(),
		ClassicScrollbars: c.classic,
		InstantScroll:     c.instant,
		NativeMenus:       c.nativeMenus,
		Theme:             (*core.Theme)(c.theme),
		ClassAliases:      c.aliases,
		Flavor:            c.flavor,
	}
	if c.accent != nil {
		cc.AccentColor, cc.HasAccentColor = *c.accent, true
	}
	return cc
}

// systemSettings returns the system settings of the headless backend.
func (c config) systemSettings() platform.SystemSettings {
	s := platform.SystemSettings{DarkMode: c.dark, ReducedMotion: c.reducedMotion, HighContrast: c.highContrast, TextScale: c.textScale}
	if c.accent != nil {
		s.AccentColor, s.HasAccentColor = c.accent.Clamp().NRGBA8(), true
	}
	return s
}

// NoSettle makes the app keep its animations where they are for
// [App.Screenshot] and [App.Golden], so a test can capture the frames of a
// transition, a [tela.UseSpring] or the enter and exit of an h.Presence:
//
//	app := telatest.Mount(t, Dialog(), telatest.NoSettle())
//	app.Click(telatest.ByRole("button", "Open"))
//	app.Advance(100 * time.Millisecond)
//	app.Golden("dialog_100ms.png") // 100 ms into the entrance
//
// With it, Mount, the events of an action, Screenshot and Golden and the
// queries [App.AriaSnapshot], [App.Count], [App.Bounds], [App.Stats] and
// [App.Cursor] handle the work due (state updates, the frames they ask
// for) but do not run the clock until running transitions and animations
// end: the clock moves with [App.Advance]. The expectations poll as ever,
// but their first try handles only the work due, and the clock moves a
// frame at a time only between failed tries. What keeps its full
// auto-wait is the wait before an action: before acting, an action waits
// for the app to settle (so the animations an earlier step started end
// first).
func NoSettle() Option { return func(c *config) { c.noSettle = true } }

// EnvCheckDamage is the environment variable that turns [CheckDamage] on
// for every app ("1").
const EnvCheckDamage = "TELA_CHECK_DAMAGE"

// CheckDamage makes the app compare its window with a full rendering after
// every frame (when nothing is left to draw), and fail the test on the
// first pixel that differs: frames are rasterized incrementally (only what
// changed, scrolled content shifted), and must give the pixels a full
// rasterization gives. It makes every frame cost a full rendering; it is
// for tests of rendering, and TELA_CHECK_DAMAGE=1 turns it on everywhere.
func CheckDamage() Option { return func(c *config) { c.checkDamage = true } }

// Size sets the logical size of the window in pixels (default 800x600).
func Size(width, height float32) Option {
	return func(c *config) { c.width, c.height = width, height }
}

// Scale sets the display scale: device pixels per logical pixel (default
// 1). Screenshots and goldens are (width*scale) x (height*scale) pixels.
func Scale(scale float64) Option { return func(c *config) { c.scale = scale } }

// Dark makes the system prefer the dark color scheme, so the dark:
// variants apply unless the app sets another scheme ([ColorScheme],
// tela.App.ColorScheme, tela.SetColorScheme).
func Dark() Option { return func(c *config) { c.dark = true } }

// ColorScheme sets the app's color scheme, like tela.App.ColorScheme:
// tela.Light or tela.Dark override the system's preference ([Dark] sets
// that one), tela.System follows it. tela.SetColorScheme changes it while
// the app runs.
func ColorScheme(s tela.ColorScheme) Option {
	if s > tela.Dark {
		panic("telatest.ColorScheme: pass tela.Light, tela.Dark or tela.System")
	}
	return func(c *config) { c.scheme = s }
}

// ReducedMotion makes the system ask for reduced motion: the
// motion-reduce: variants apply, transitions take no time and scrolling
// jumps.
func ReducedMotion() Option { return func(c *config) { c.reducedMotion = true } }

// HighContrast makes the system ask for high contrast: the
// contrast-more: variants apply and tela.UseHighContrast reports true.
func HighContrast() Option { return func(c *config) { c.highContrast = true } }

// TextScale sets the system's text scale (default 1): 1rem is the theme's
// rem times scale, so text and rem-based spacing grow (tela.UseTextScale),
// unless the app sets tela.App.IgnoreTextScale. The scale is above 0 and
// at most 4.
func TextScale(scale float64) Option { return func(c *config) { c.textScale = scale } }

// AccentColor sets the system's accent color, which tela.UseAccentColor
// returns (by default the system has none).
func AccentColor(color tela.Color) Option { return func(c *config) { c.accent = &color } }

// ClassicScrollbars draws always-visible classic scrollbars that take
// space (tela.App.ClassicScrollbars).
func ClassicScrollbars() Option { return func(c *config) { c.classic = true } }

// InstantScroll makes user scrolling jump, without animation or flings
// (tela.App.InstantScroll).
func InstantScroll() Option { return func(c *config) { c.instant = true } }

// PreferNativeMenus makes kit's ContextMenu and Menubar use the OS's menus
// where the platform has them (tela.App.NativeMenus, ADR-215). The headless
// desktop has them, unless [App.Unsupported] takes them away
// ([NativeMenus]) or the window is [Frameless] (no menu bar). Answer a
// native context menu with [App.ChoosePopupItem] and read a native bar
// with [App.MenuBar].
func PreferNativeMenus() Option { return func(c *config) { c.nativeMenus = true } }

// PlatformKeys makes the app follow the key conventions of the platform
// goos names, a runtime.GOOS value: "darwin" for macOS, "windows" for
// Windows, anything else for Linux (ADR-221). The key bindings and word
// rules of text fields follow it (Command+A selects all, Option+Left moves
// by word under "darwin"), and so do the events' CmdOrCtrl and the aria
// hooks built on it (Command+click toggles a selection on a Mac), and
// [App.Press] and [Modifiers] resolve "ControlOrMeta" to Meta under
// "darwin" and to Control otherwise. Without it the app follows the OS the
// test runs on. It applies to one app and its windows, so parallel tests
// with different platforms do not interfere:
//
//	app := telatest.Mount(t, Editor(), telatest.PlatformKeys("darwin"))
//	app.Press("ControlOrMeta+a") // Meta+a: selects all
//	app.Press("Alt+ArrowLeft")   // Option+Left: one word back
//
// Only the conventions change: the headless backend, the look of the app
// and what the app reads from runtime.GOOS stay those of the host.
func PlatformKeys(goos string) Option {
	fl := platform.FlavorOf(goos)
	return func(c *config) { c.flavor = fl }
}

// devMode returns whether the tree runs in dev mode: TELA_DEV=1 or
// TELA_DEV=0 overrides the option.
func (c config) devMode() bool { return core.ResolveDev(&c.dev) }

// NoDev turns dev mode off. Apps and the headless entry points run in dev
// mode by default, as a development build of an app does: unknown
// classes, roles, input types and easings, hooks called in another order
// or outside their component, and the dev warnings (lists without keys,
// nil dependencies, a Value read while changes are queued, a Label
// without a control...) are reported. With [Mount], a dev error (an
// unknown class, an image that cannot be loaded) fails the test at the
// action that caused it unless [AllowDev] lists it, a dev panic fails it
// as any panic does, and a warning is a test log line; with [Render] and
// the other entry points, errors and warnings go to standard error.
//
// Use NoDev to test what a release build does. The checks of the node
// constructors (h.If with a struct component, Role, Type, Dir...) follow
// the app's dev mode when they run while a component builds, as in a
// function given to [MountFunc]; a node built outside any build (the root
// passed to [Mount] in the test's body) follows the process default,
// which is on in a test binary. TELA_DEV=1 or TELA_DEV=0 in the
// environment overrides both NoDev and the default.
func NoDev() Option { return func(c *config) { c.dev = false } }

// Dev turns dev mode on. It is the default since dev mode became the
// default (see [NoDev]), so Dev changes nothing; it is kept for the tests
// written before, and undoes a [NoDev] given earlier in the options.
func Dev() Option { return func(c *config) { c.dev = true } }

// AllowDev lists dev errors that do not fail a test: those containing any
// of the substrings, which are logged instead. Dev warnings never fail a
// test, and AllowDev does nothing under [NoDev].
func AllowDev(substrings ...string) Option {
	return func(c *config) { c.allowDev = append(c.allowDev, substrings...) }
}

// RootClass sets the classes of the root box, which fills the window
// (tela.App.RootClass): a background, a text style, overflow-hidden.
func RootClass(classes string) Option { return func(c *config) { c.rootClass = classes } }

// Timeout sets how long actions and expectations wait for their condition
// before they fail the test (default [DefaultTimeout]). It is wall-clock
// time, as in Playwright, while the app's clock moves one frame at a time
// during the wait. It also bounds, in app time (up to [DefaultTimeout]),
// how long a single action lets the app run before it stops waiting for
// it to go quiet (an animation that never ends is logged, not waited
// for); a finite animation runs to its end however slow the machine is.
func Timeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// Debug turns debug overlays on: any of "layout", "paint-flash", "layers",
// "fps" and "semantics" (or "all"), the values TELA_DEBUG takes. The
// overlays are drawn over the window, so [App.Screenshot] and
// [App.Golden] show them; the headless entry points ([Render]) draw them
// too. TELA_DEBUG itself is ignored, so goldens never depend on the
// environment. An unknown name fails the test with the valid ones.
//
// A paint flash fades over 300 ms of app time, and the fps meter redraws on
// every frame; neither keeps an action waiting (they are not work the app
// must finish), so look at them with [App.Advance]. See [DebugClock] for
// the frame times the meter shows.
func Debug(flags ...string) Option {
	return func(c *config) { c.debug = append(c.debug, flags...) }
}

// DebugClock replaces the clock the fps overlay times frames with: a frame
// took the difference between two readings of now, taken before and after
// it. The default reads 1 ms longer at every call, so every frame takes
// 1 ms and goldens do not depend on the machine; tests of the meter pass a
// scripted clock.
func DebugClock(now func() time.Duration) Option {
	return func(c *config) { c.debugClock = now }
}

// Theme sets the app's theme, like tela.App.Theme: the color tokens
// (bg-primary, text-muted-foreground), the base radius and the fonts.
// Without it the app has tela.DefaultTheme. tela.SetTheme changes it
// while the app runs.
func Theme(t tela.Theme) Option {
	t.Colors = maps.Clone(t.Colors) // the caller may reuse its map
	return func(c *config) { c.theme = &t }
}

// ClassAliases sets the app's class aliases, like tela.App.ClassAliases:
// class names that expand to class strings where they appear.
func ClassAliases(aliases map[string]string) Option {
	aliases = maps.Clone(aliases)
	return func(c *config) { c.aliases = aliases }
}

// Fonts adds fonts to the app, before the deterministic set every app has
// (the fonts of fonts/standard: Inter, JetBrains Mono, Source Serif 4,
// Noto Sans Arabic and Noto Color Emoji), like tela.App.Fonts: they win the
// generic families they stand for (a Sans() font replaces Inter as
// font-sans) and are tried first in fallback. Tests never use the
// system's fonts, so goldens are the same on every machine. A font that
// cannot be used (a FontFS pattern that matches nothing, a file that is
// not a font) fails the test at Mount.
//
//	app := telatest.Mount(t, Notes(), telatest.Fonts(tela.FontData("Lora", lora).Serif()))
func Fonts(fonts ...tela.Font) Option {
	fonts = append([]tela.Font(nil), fonts...)
	return func(c *config) { c.fonts = append(c.fonts, fonts...) }
}

// FromApp applies the settings of a [tela.App] that shape what is drawn, so
// a test, `tela snapshot` and the window see the same app: the size when
// both Width and Height are set, RootClass, ColorScheme, Theme,
// ClassAliases, Fonts, ClassicScrollbars, InstantScroll, NativeMenus,
// IgnoreTextScale and Dev. It sets only
// what the app sets, so it never undoes an earlier option, and the options
// after it override it:
//
//	var App = tela.App{Title: "Notes", Width: 900, Height: 600, RootClass: "bg-background", Root: Notes()}
//
//	app := telatest.Mount(t, Notes(), telatest.FromApp(App))
//	app := telatest.Mount(t, Notes{}, telatest.FromApp(App), telatest.Size(400, 300))
//
// Title, AppID, Root, DevState and the fields of the OS window (size
// limits, FixedSize, Frameless, AlwaysOnTop, Hidden, Icon, Placement,
// OnClose) say nothing about how the content looks, so FromApp ignores
// them: tools that only render the app (`tela snapshot`, `tela mcp`) want
// it visible and quit-able whatever the app does with its window.
// [MountApp] mounts Root and applies those fields too, as the options
// [Title], [MinSize], [Frameless], [StartHidden], [Placement], [OnClose]
// and the others do by hand. The scale is not an App setting (it comes
// from the display): use [Scale].
func FromApp(app tela.App) Option {
	if app.ColorScheme > tela.Dark {
		panic("telatest.FromApp: App.ColorScheme must be tela.Light, tela.Dark or tela.System")
	}
	aliases := maps.Clone(app.ClassAliases)
	fonts := append([]tela.Font(nil), app.Fonts...)
	var theme *tela.Theme
	if app.Theme != nil {
		t := *app.Theme
		t.Colors = maps.Clone(t.Colors)
		theme = &t
	}
	return func(c *config) {
		if app.Width > 0 && app.Height > 0 {
			c.width, c.height = app.Width, app.Height
		}
		if app.RootClass != "" {
			c.rootClass = app.RootClass
		}
		if app.ColorScheme != tela.System {
			c.scheme = app.ColorScheme
		}
		if theme != nil {
			c.theme = theme
		}
		if aliases != nil {
			c.aliases = aliases
		}
		c.fonts = append(c.fonts, fonts...)
		if app.ClassicScrollbars {
			c.classic = true
		}
		if app.InstantScroll {
			c.instant = true
		}
		if app.NativeMenus {
			c.nativeMenus = true
		}
		if app.IgnoreTextScale {
			c.ignoreText = true
		}
		if app.Dev != nil {
			c.dev = *app.Dev
		}
	}
}

// finiteRect reports whether r's coordinates are numbers within the range
// the headless window manager accepts (it panics beyond ±1e7).
func finiteRect(r tela.Rect) bool {
	for _, v := range [4]float32{r.X, r.Y, r.Width, r.Height} {
		if v != v || v > 1e7 || v < -1e7 {
			return false
		}
	}
	return true
}
