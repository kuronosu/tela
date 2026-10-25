package i18n

import (
	"fmt"

	"github.com/kuronosu/tela"
)

// localeContext carries a locale chosen by [Provider]; "" means none.
var localeContext = tela.CreateContext("")

// Provider makes the locale tag the locale of its children: [UseLocale],
// [UseT] and [UseFormatter] inside it return that locale instead of the
// current one, whatever [SetLocale] says. Components below it build again
// when the tag changes between builds of the Provider.
//
// It exists for isolation. A test mounts a tree under a Provider and does
// not touch the process-wide locale, so parallel tests cannot see each
// other's, and an app can show a preview in another language:
//
//	app := telatest.Mount(t, i18n.Provider("es", Settings()))
//
// Provider("") has no effect: its children follow the current locale. It
// panics if tag is not a valid BCP 47 tag.
func Provider(tag string, children ...tela.Child) tela.Node {
	name := ""
	if tag != "" {
		d, err := forTag(tag)
		if err != nil {
			panic("i18n: Provider(" + fmt.Sprintf("%q", tag) + "): " + err.Error())
		}
		name = d.name
	}
	return localeContext.Provider(name, children...)
}

// UseLocale returns the current locale ([Locale], or the one of the
// nearest [Provider]) and subscribes the component to it: when
// [SetLocale] changes the locale, the component builds again, and only the
// components that called UseLocale, [UseT] or [UseFormatter] do. It is a
// hook: call it at the top level of a component.
//
//	func Clock() tela.Node {
//	    loc := i18n.UseLocale() // rebuilds on a language change
//	    return Text(i18n.FormatTime(time.Now(), i18n.Short))
//	}
//
// The package-level [FormatNumber], [FormatDate] and the like use the
// current locale without subscribing: pair them with UseLocale, or use
// the methods of [UseFormatter].
func UseLocale() string {
	provided := tela.UseContext(localeContext)
	cur, set := tela.UseState(Locale())
	tela.UseEffect(func() func() {
		if provided != "" {
			return nil
		}
		// The setter is safe from any goroutine, and Update applies on
		// the UI thread reading the locale at that moment, so changes
		// that race each other end on the latest one.
		catchUp := func() { set.Update(func(string) string { return Locale() }) }
		unsubscribe := subscribe(catchUp)
		catchUp() // a change between this build and the effect
		return unsubscribe
	}, provided)
	if provided != "" {
		return provided
	}
	return cur
}

// UseFormatter returns the [Formatter] of the component's locale and
// subscribes the component to it, like [UseLocale]. Use it for numbers,
// dates and lists in components, together with messages:
//
//	f := i18n.UseFormatter()
//	Text(f.T("updated", "when", f.Date(t, i18n.Medium)))
//
// It is a hook: call it at the top level of a component.
func UseFormatter() Formatter { return For(UseLocale()) }

// UseT returns the translator of the component's locale and subscribes
// the component to it, like [UseLocale]: switching the locale builds the
// component again with the new language. Its arguments are name/value
// pairs; see [Formatter.T]. It is a hook: call it at the top level of a
// component.
//
//	func Greeting(name string) tela.Node {
//	    t := i18n.UseT()
//	    return H1(Text(t("hello", "name", name)))
//	}
//
// In handlers and goroutines, where hooks cannot be called, use [T].
func UseT() func(key string, args ...any) string { return UseFormatter().T }
