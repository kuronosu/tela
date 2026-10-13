package telatest

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/core"
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
	classic       bool
	instant       bool
	dev           bool
	allowDev      []string
	rootClass     string
	timeout       time.Duration
	checkDamage   bool
	debug         []string
	debugClock    func() time.Duration
}

func newConfig(opts []Option) config {
	cfg := config{scale: 1, timeout: DefaultTimeout, checkDamage: os.Getenv(EnvCheckDamage) == "1"}
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
	}
	if _, err := core.ParseDebug(strings.Join(c.debug, ",")); err != nil {
		return fmt.Errorf("telatest.Debug: %v", err)
	}
	return nil
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
	return core.Config{
		Debug:             dbg,
		DebugClock:        clock,
		RootClass:         c.rootClass,
		Dark:              c.dark,
		ReducedMotion:     c.reducedMotion,
		Dev:               c.dev,
		ClassicScrollbars: c.classic,
		InstantScroll:     c.instant,
	}
}

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
// variants apply.
func Dark() Option { return func(c *config) { c.dark = true } }

// ReducedMotion makes the system ask for reduced motion: the
// motion-reduce: variants apply, transitions take no time and scrolling
// jumps.
func ReducedMotion() Option { return func(c *config) { c.reducedMotion = true } }

// ClassicScrollbars draws always-visible classic scrollbars that take
// space (tela.App.ClassicScrollbars).
func ClassicScrollbars() Option { return func(c *config) { c.classic = true } }

// InstantScroll makes user scrolling jump, without animation or flings
// (tela.App.InstantScroll).
func InstantScroll() Option { return func(c *config) { c.instant = true } }

// Dev turns dev mode on: unknown classes, lists without keys, hook-order
// mistakes and the other dev messages of the framework. With [Mount],
// every dev message fails the test (at the action that triggered it)
// unless [AllowDev] lists it. With [Render] and the other entry points,
// the messages go to standard error.
func Dev() Option { return func(c *config) { c.dev = true } }

// AllowDev lists dev messages that do not fail a test under [Dev]: those
// containing any of the substrings. It has no effect without [Dev].
func AllowDev(substrings ...string) Option {
	return func(c *config) { c.allowDev = append(c.allowDev, substrings...) }
}

// RootClass sets the classes of the root box, which fills the window
// (tela.App.RootClass): a background, a text style, overflow-hidden.
func RootClass(classes string) Option { return func(c *config) { c.rootClass = classes } }

// Timeout sets how long actions and expectations wait for their condition
// before they fail the test (default [DefaultTimeout]). It is wall-clock
// time, as in Playwright, while the app's clock moves one frame at a time
// during the wait; it also bounds how long a single action spends waiting
// for the app to go quiet (an animation that never ends is logged, not
// waited for).
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
