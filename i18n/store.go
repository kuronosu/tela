package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/text/language"

	"github.com/kuronosu/tela/internal/core"
)

// The process-wide state: the loaded catalogs, the locale and the
// fallback. Like tela.Assets, it is a global registry: an app loads its
// catalogs once, from init or main. It is an immutable snapshot swapped
// atomically, so reads (T, Locale, the hooks) never lock, and writers
// (Load, SetLocale, SetFallback) are safe from any goroutine.

// catalog is the messages of one locale.
type catalog struct {
	loc  *locData
	msgs map[string][]node
}

type state struct {
	cats        map[string]*catalog // by canonical locale name
	order       []string            // names in load order
	fallbackSet string              // fallback set with SetFallback ("" for the default)
	explicit    string              // locale set with SetLocale ("" follows the system)

	// computed by build
	fallback string // name of the fallback catalog ("" with none)
	locale   string // the current locale

	// matching: catalog locales by tag, and a matcher over them
	tags    []language.Tag // by load order
	byTag   map[language.Tag]int
	matcher language.Matcher

	chains sync.Map // locale name -> []*catalog
}

var (
	stMu sync.Mutex // serializes writers
	st   atomic.Pointer[state]

	subsMu sync.Mutex
	subs   = map[uint64]func(){}
	subSeq uint64
)

func init() { st.Store(build(&state{cats: map[string]*catalog{}})) }

func current() *state { return st.Load() }

// update applies f to a copy of the state and publishes it, then tells
// the subscribers if the current locale changed.
func update(f func(s *state) error) error {
	stMu.Lock()
	old := st.Load()
	ns := &state{
		cats:        make(map[string]*catalog, len(old.cats)),
		order:       append([]string(nil), old.order...),
		fallbackSet: old.fallbackSet,
		explicit:    old.explicit,
	}
	for k, v := range old.cats {
		ns.cats[k] = v
	}
	if err := f(ns); err != nil {
		stMu.Unlock()
		return err
	}
	build(ns)
	st.Store(ns)
	stMu.Unlock()
	if ns.locale != old.locale {
		notify()
	}
	return nil
}

// build completes a state: matching structures, the fallback and the
// current locale.
func build(s *state) *state {
	s.byTag = make(map[language.Tag]int, len(s.cats))
	s.tags = make([]language.Tag, 0, len(s.order))
	for i, n := range s.order {
		t := s.cats[n].loc.tag
		s.byTag[t] = i
		s.tags = append(s.tags, t)
	}
	if len(s.tags) > 0 {
		s.matcher = language.NewMatcher(s.tags)
	}
	switch {
	case s.fallbackSet != "":
		s.fallback = s.fallbackSet
	case s.cats["en"] != nil:
		s.fallback = "en"
	case len(s.order) > 0:
		s.fallback = s.order[0]
	default:
		s.fallback = ""
	}
	switch {
	case s.explicit != "":
		s.locale = s.explicit
	default:
		sys, ok := systemLocale()
		switch {
		case ok && len(s.cats) > 0 && s.match(sys) != nil:
			s.locale = sys.String()
		case s.fallback != "":
			s.locale = s.fallback
		case ok:
			s.locale = sys.String()
		default:
			s.locale = "en"
		}
	}
	return s
}

// match finds the catalog for a tag: its own, a CLDR parent's, or the
// closest of the same language; nil if there is none.
func (s *state) match(t language.Tag) *catalog {
	if s.matcher == nil {
		return nil
	}
	i, ok := resolveTag(t, s.byTag, s.matcher, s.tags)
	if !ok {
		return nil
	}
	return s.cats[s.order[i]]
}

// chain returns the catalogs searched for a message of the locale, best
// first: the locale's catalog, its parents', the fallback's.
func (s *state) chain(d *locData) []*catalog {
	if v, ok := s.chains.Load(d.name); ok {
		return v.([]*catalog)
	}
	var out []*catalog
	add := func(c *catalog) {
		if c == nil {
			return
		}
		for _, o := range out {
			if o == c {
				return
			}
		}
		out = append(out, c)
	}
	if c := s.match(d.tag); c != nil {
		add(c)
		for t := c.loc.tag.Parent(); t != language.Und; t = t.Parent() {
			if i, ok := s.byTag[t]; ok {
				add(s.cats[s.order[i]])
			}
		}
	}
	add(s.cats[s.fallback])
	s.chains.Store(d.name, out)
	return out
}

// currentLoc returns the data of the current locale.
func currentLoc() *locData {
	l := current().locale
	d, err := forTag(l)
	if err != nil {
		d, _ = forTag("en")
	}
	return d
}

// Locale returns the current locale as a BCP 47 tag ("en-US", "es"): the
// one set with [SetLocale], or else the system's locale when a loaded
// catalog covers its language, or else the fallback locale. It is a
// string, not an x/text type, so that apps do not depend on that module.
// Components should use [UseLocale], which also rebuilds them when the
// locale changes.
func Locale() string { return current().locale }

// Locales returns the locales of the loaded catalogs, in load order.
func Locales() []string { return append([]string(nil), current().order...) }

// SetLocale sets the current locale, for example "es", "pt-BR" or "ar"
// (underscores are accepted: "es_MX"). The components that read the
// locale with [UseLocale], [UseT] or [UseFormatter] build again, and only
// those; [T] and the Format functions use it from then on. An empty tag
// follows the system's locale again. It is safe from any goroutine.
//
// The tag need not have a catalog: messages then come from the nearest
// one (es-MX uses es) or the fallback locale, while numbers, dates and
// lists are formatted for the tag itself. It panics if tag is not a valid
// BCP 47 tag.
func SetLocale(tag string) {
	name := ""
	if strings.TrimSpace(tag) != "" {
		d, err := forTag(tag)
		if err != nil {
			panic("i18n: SetLocale(" + fmt.Sprintf("%q", tag) + "): " + err.Error())
		}
		name = d.name
	}
	_ = update(func(s *state) error { s.explicit = name; return nil })
}

// SetFallback sets the locale whose messages are used when the current
// locale's catalogs lack a key, and when the system locale is not
// covered. By default it is "en" if a catalog for it is loaded, else the
// first catalog loaded (the first file, in file name order, of the first
// [Load]). The locale must be loaded.
func SetFallback(tag string) error {
	d, err := forTag(tag)
	if err != nil {
		return errors.New("i18n: SetFallback(" + fmt.Sprintf("%q", tag) + "): " + err.Error())
	}
	return update(func(s *state) error {
		if _, ok := s.cats[d.name]; !ok {
			return fmt.Errorf("i18n: SetFallback(%q): no catalog for %q is loaded (loaded: %s); call Load first", tag, d.name, strings.Join(s.order, ", "))
		}
		s.fallbackSet = d.name
		return nil
	})
}

// ---------------------------------------------------------------------
// Loading.

// Load reads the message catalogs that match pattern in fsys. Each file
// is a JSON object of messages in ICU MessageFormat, named after its
// locale: locales/en-US.json is the locale "en-US", locales/es.json "es".
//
//	{
//	  "hello": "Hello, {name}!",
//	  "files": "{count, plural, one {# file} other {# files}}",
//	  "nav": {"home": "Home"}
//	}
//
// Nested objects give dotted keys ("nav.home"). Call it from init or main,
// with an embedded file system:
//
//	//go:embed locales/*.json
//	var locales embed.FS
//
//	func init() { must(i18n.Load(locales, "locales/*.json")) }
//
// Load may be called several times: a locale loaded again adds its
// messages, and a key that is already there is replaced. Every message is
// parsed at load time, so a syntax error is reported here, with the file,
// the key and the position, and nothing is loaded. The fallback locale
// is "en" when loaded, else the first catalog loaded, unless
// [SetFallback] says otherwise. It is safe from any goroutine.
func Load(fsys fs.FS, pattern string) error {
	files, err := fs.Glob(fsys, pattern)
	if err != nil {
		return fmt.Errorf("i18n: Load: bad pattern %q: %v", pattern, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("i18n: Load: pattern %q matches no files; check the pattern and, for an embedded file system, the //go:embed directive", pattern)
	}
	type loaded struct {
		loc  *locData
		msgs map[string][]node
	}
	var all []loaded
	var errs []error
	for _, name := range files {
		base := strings.TrimSuffix(path.Base(name), path.Ext(name))
		d, err := forTag(base)
		if err != nil {
			errs = append(errs, fmt.Errorf("i18n: %s: the file name must be a locale such as en.json, es-MX.json or pt-BR.json: %v", name, err))
			continue
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("i18n: %v", err))
			continue
		}
		msgs, perrs := parseCatalog(name, data)
		errs = append(errs, perrs...)
		all = append(all, loaded{d, msgs})
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return update(func(s *state) error {
		for _, l := range all {
			c := s.cats[l.loc.name]
			if c == nil {
				c = &catalog{loc: l.loc, msgs: map[string][]node{}}
				s.order = append(s.order, l.loc.name)
			} else {
				// Copy: the old state may still be read.
				nc := &catalog{loc: c.loc, msgs: make(map[string][]node, len(c.msgs)+len(l.msgs))}
				for k, v := range c.msgs {
					nc.msgs[k] = v
				}
				c = nc
			}
			for k, v := range l.msgs {
				c.msgs[k] = v
			}
			s.cats[l.loc.name] = c
		}
		return nil
	})
}

// parseCatalog reads a catalog file: nested objects of ICU messages.
func parseCatalog(file string, data []byte) (map[string][]node, []error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, []error{fmt.Errorf("i18n: %s: %v (a catalog is a JSON object of messages)", file, err)}
	}
	msgs := map[string][]node{}
	var errs []error
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key := prefix + k
			switch v := m[k].(type) {
			case string:
				nodes, err := parseMessage(v)
				if err != nil {
					errs = append(errs, fmt.Errorf("i18n: %s: message %q: %v", file, key, err))
					continue
				}
				msgs[key] = nodes
			case map[string]any:
				walk(key+".", v)
			default:
				errs = append(errs, fmt.Errorf("i18n: %s: message %q must be a string (an ICU message) or an object of messages, found %T", file, key, v))
			}
		}
	}
	walk("", raw)
	return msgs, errs
}

// ---------------------------------------------------------------------
// Subscriptions: how components learn that the locale changed.

// subscribe registers fn, called (on the goroutine that changed the
// locale) after the current locale changes; it returns the function that
// removes it.
func subscribe(fn func()) (unsubscribe func()) {
	subsMu.Lock()
	subSeq++
	id := subSeq
	subs[id] = fn
	subsMu.Unlock()
	return func() {
		subsMu.Lock()
		delete(subs, id)
		subsMu.Unlock()
	}
}

func notify() {
	subsMu.Lock()
	fns := make([]func(), 0, len(subs))
	for _, fn := range subs {
		fns = append(fns, fn)
	}
	subsMu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// ---------------------------------------------------------------------
// Dev mode.

// devMode reports whether problems are reported (ADR-085): TELA_DEV=1 or
// TELA_DEV=0 when set (read at each call, so that a test can change it),
// else Tela's dev mode at the call (core.DevNow): the mode of the Tree
// being built when T runs in a component, else the process default, on
// unless the binary is a release build and the app's mode (App.Dev) once
// Run starts.
func devMode() bool {
	switch os.Getenv(core.EnvDev) {
	case "1":
		return true
	case "0":
		return false
	}
	return core.DevNow()
}

var (
	devOut  func(string) = func(s string) { fmt.Fprintln(os.Stderr, s) }
	devSeen sync.Map
)

func devWarn(msg string) { devOut(msg) }

func devWarnOnce(key, msg string) {
	if _, dup := devSeen.LoadOrStore(key, struct{}{}); !dup {
		devOut(msg)
	}
}

// devReportMissing reports a key found in no catalog, once per locale and
// key, in dev mode.
func devReportMissing(s *state, d *locData, key string) {
	if !devMode() {
		return
	}
	var tried []string
	for _, c := range s.chain(d) {
		tried = append(tried, c.loc.name)
	}
	where := "no catalog is loaded; call i18n.Load first"
	if len(tried) > 0 {
		where = "searched " + strings.Join(tried, ", ")
	}
	devWarnOnce("missing\x00"+d.name+"\x00"+key,
		fmt.Sprintf("tela: i18n: no message %q for locale %s (%s); add it to the catalog of %s", key, d.name, where, d.name))
}
