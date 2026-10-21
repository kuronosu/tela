package i18n_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// These tests share the process-wide catalogs and locale, so none of them
// is parallel; they load the same catalogs and leave the locale at its
// default.

var testCatalogs = fstest.MapFS{
	"locales/en.json": {Data: []byte(`{"greeting": "Hello, {name}!", "files": "{count, plural, =0 {No files} one {# file} other {# files}}", "title": "Settings"}`)},
	"locales/es.json": {Data: []byte(`{"greeting": "¡Hola, {name}!", "files": "{count, plural, =0 {Sin archivos} one {# archivo} other {# archivos}}", "title": "Ajustes"}`)},
	"locales/ar.json": {Data: []byte(`{"greeting": "مرحبا، {name}!", "files": "{count, plural, zero {لا ملفات} one {ملف} two {ملفان} few {# ملفات} many {# ملفًا} other {# ملف}}", "title": "الإعدادات"}`)},
}

func load(t *testing.T) {
	t.Helper()
	if err := i18n.Load(testCatalogs, "locales/*.json"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLocale("en") })
	i18n.SetLocale("en")
}

var (
	titleBuilds, bodyBuilds, plainBuilds, staticBuilds, parentBuilds atomic.Int32
)

func Title() tela.Node {
	titleBuilds.Add(1)
	t := i18n.UseT()
	return h.H1(h.Text(t("title")))
}

func Body() tela.Node {
	bodyBuilds.Add(1)
	loc := i18n.UseLocale()
	return h.P(h.Textf("locale:%s %s", loc, i18n.FormatNumber(1234.5)))
}

// Plain calls T without a hook: it never rebuilds on a locale change.
func Plain() tela.Node {
	plainBuilds.Add(1)
	return h.P(h.Text(i18n.T("title")))
}

func Static() tela.Node {
	staticBuilds.Add(1)
	return h.P(h.Text("static"))
}

func App() tela.Node {
	parentBuilds.Add(1)
	return h.Div(tela.C(Title), tela.C(Body), tela.C(Plain), tela.C(Static))
}

// SetLocale rebuilds only the components that read the locale.
func TestSetLocaleRebuildsOnlySubscribers(t *testing.T) {
	load(t)
	app := telatest.Mount(t, tela.C(App))
	app.ExpectText("Settings")
	app.ExpectText("locale:en 1,234.5")
	for _, c := range []*atomic.Int32{&titleBuilds, &bodyBuilds, &plainBuilds, &staticBuilds, &parentBuilds} {
		c.Store(0)
	}

	i18n.SetLocale("es")
	app.ExpectText("Ajustes")
	app.ExpectText("locale:es 1.234,5")
	if got := titleBuilds.Load(); got != 1 {
		t.Errorf("UseT component built %d times, want 1", got)
	}
	if got := bodyBuilds.Load(); got != 1 {
		t.Errorf("UseLocale component built %d times, want 1", got)
	}
	if plainBuilds.Load() != 0 || staticBuilds.Load() != 0 || parentBuilds.Load() != 0 {
		t.Errorf("components without UseT or UseLocale built: plain %d, static %d, parent %d",
			plainBuilds.Load(), staticBuilds.Load(), parentBuilds.Load())
	}
	// The unsubscribed component still shows the old language.
	app.ExpectText("Settings")

	// Setting the same locale builds nothing.
	titleBuilds.Store(0)
	bodyBuilds.Store(0)
	i18n.SetLocale("es")
	app.Advance(50 * time.Millisecond)
	if titleBuilds.Load() != 0 || bodyBuilds.Load() != 0 {
		t.Errorf("same locale rebuilt: %d, %d", titleBuilds.Load(), bodyBuilds.Load())
	}

	i18n.SetLocale("en")
	app.ExpectText("locale:en 1,234.5")
	app.ExpectNoText("Ajustes")
}

// SetLocale from another goroutine.
func TestSetLocaleFromGoroutine(t *testing.T) {
	load(t)
	app := telatest.Mount(t, tela.C(Title))
	app.ExpectText("Settings")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); i18n.SetLocale("es") }()
	wg.Wait()
	app.ExpectText("Ajustes")
}

// A handler switches the language, the usual way.
func TestSwitchFromHandler(t *testing.T) {
	load(t)
	root := func() tela.Node {
		t := i18n.UseT()
		return h.Div(
			h.H1(h.Text(t("greeting", "name", "Ana"))),
			h.P(h.Text(t("files", "count", 3))),
			h.Button(h.OnClick(func(tela.MouseEvent) { i18n.SetLocale("es") }), h.Text("Español")),
			h.Button(h.OnClick(func(tela.MouseEvent) { i18n.SetLocale("ar") }), h.Text("العربية")),
		)
	}
	app := telatest.Mount(t, tela.C(root))
	app.ExpectText("Hello, Ana!")
	app.ExpectText("3 files")
	app.Click(telatest.ByRole("button", "Español"))
	app.ExpectText("¡Hola, Ana!")
	app.ExpectText("3 archivos")
	app.Click(telatest.ByRole("button", "العربية"))
	app.ExpectText("٣ ملفات")
}

func Greeting() tela.Node {
	f := i18n.UseFormatter()
	return h.P(h.Textf("%s | %s | %s | %s", f.T("greeting", "name", "Ana"), f.Locale(), f.Number(1234.5), f.Dir()))
}

// Provider fixes the locale of a subtree whatever the global locale is.
func TestProviderIsolatesSubtrees(t *testing.T) {
	load(t)
	root := func() tela.Node {
		return h.Div(
			i18n.Provider("es", tela.C(Greeting)),
			i18n.Provider("ar", tela.C(Greeting)),
			tela.C(Greeting),
		)
	}
	app := telatest.Mount(t, tela.C(root))
	app.ExpectText("¡Hola, Ana! | es | 1.234,5 | LTR")
	app.ExpectText("مرحبا، Ana! | ar | ١٬٢٣٤٫٥ | RTL")
	app.ExpectText("Hello, Ana! | en | 1,234.5 | LTR")

	// The global locale moves only the tree that is not under a Provider.
	i18n.SetLocale("es")
	app.ExpectCount(telatest.ByText("¡Hola, Ana! | es | 1.234,5 | LTR"), 2)
	app.ExpectText("مرحبا، Ana! | ar | ١٬٢٣٤٫٥ | RTL")
	app.ExpectNoText("Hello, Ana!")
}

// A Provider whose tag changes between builds moves its readers.
func TestProviderTagChanges(t *testing.T) {
	load(t)
	root := func() tela.Node {
		loc, setLoc := tela.UseState("en")
		return h.Div(
			h.Button(h.OnClick(func(tela.MouseEvent) { setLoc.Set("es") }), h.Text("switch")),
			i18n.Provider(loc, tela.C(Title)),
		)
	}
	app := telatest.Mount(t, tela.C(root))
	app.ExpectText("Settings")
	app.Click(telatest.ByRole("button", "switch"))
	app.ExpectText("Ajustes")
}

func TestProviderEmptyFollowsCurrent(t *testing.T) {
	load(t)
	app := telatest.Mount(t, i18n.Provider("", tela.C(Title)))
	app.ExpectText("Settings")
	i18n.SetLocale("es")
	app.ExpectText("Ajustes")
}

func TestProviderInvalidTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("no panic for an invalid tag")
		}
	}()
	i18n.Provider("not a tag!")
}
