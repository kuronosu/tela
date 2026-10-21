package i18n

import (
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

var enES = map[string]string{
	"en.json": `{"hello": "Hello, {name}!", "files": "{count, plural, one {# file} other {# files}}", "only_en": "Only English", "nav": {"home": "Home", "deep": {"x": "X"}}}`,
	"es.json": `{"hello": "¡Hola, {name}!", "files": "{count, plural, one {# archivo} other {# archivos}}", "nav": {"home": "Inicio"}}`,
}

func TestLoadAndTranslate(t *testing.T) {
	isolate(t)
	mustLoad(t, enES)
	if got := Locales(); len(got) != 2 || got[0] != "en" || got[1] != "es" {
		t.Errorf("Locales = %v", got)
	}
	SetLocale("es")
	if got := T("hello", "name", "Ana"); got != "¡Hola, Ana!" {
		t.Errorf("T hello = %q", got)
	}
	if got := T("files", "count", 1); got != "1 archivo" {
		t.Errorf("T files = %q", got)
	}
	if got := T("nav.home"); got != "Inicio" {
		t.Errorf("nested key = %q", got)
	}
	// Missing in es: the fallback (en, the first loaded) answers.
	if got := T("only_en"); got != "Only English" {
		t.Errorf("fallback = %q", got)
	}
	if got := T("nav.deep.x"); got != "X" {
		t.Errorf("deep key from fallback = %q", got)
	}
	SetLocale("en-US")
	if got := T("hello", "name", "Bob"); got != "Hello, Bob!" {
		t.Errorf("en-US = %q", got)
	}
}

func TestFallbackChain(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{
		"en.json":    `{"a": "en a", "b": "en b", "c": "en c"}`,
		"es.json":    `{"a": "es a", "b": "es b"}`,
		"es-MX.json": `{"a": "es-MX a"}`,
	})
	SetLocale("es-MX")
	for key, want := range map[string]string{"a": "es-MX a", "b": "es b", "c": "en c", "d": "d"} {
		if got := T(key); got != want {
			t.Errorf("es-MX %s = %q, want %q", key, got, want)
		}
	}
	// es-AR has no catalog: es-419 and es-MX are not its parents, so it
	// finds es.
	SetLocale("es-AR")
	if got := T("a"); got != "es a" {
		t.Errorf("es-AR = %q", got)
	}
	// A language with no catalog uses the fallback.
	SetLocale("fr")
	if got := T("b"); got != "en b" {
		t.Errorf("fr = %q", got)
	}
	if err := SetFallback("es"); err != nil {
		t.Fatal(err)
	}
	if got := T("b"); got != "es b" {
		t.Errorf("fr with es fallback = %q", got)
	}
	// c is only in en, which is no longer in the chain.
	if got := T("c"); got != "c" {
		t.Errorf("c with es fallback = %q", got)
	}
}

func TestFallbackLanguageDrivesPlurals(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"en.json": `{"files": "{count, plural, one {# file} other {# files}}"}`})
	SetLocale("ru")
	// The message is English: English plural rules (1 is one, 21 is other)
	// and numbers formatted as the English text expects.
	if got := T("files", "count", 21); got != "21 files" {
		t.Errorf("ru with English message = %q", got)
	}
	if got := T("files", "count", 1234); got != "1,234 files" {
		t.Errorf("ru number in English message = %q", got)
	}
}

func TestMissingKey(t *testing.T) {
	log := isolate(t)
	mustLoad(t, enES)
	SetLocale("es")
	if got := T("nope"); got != "nope" {
		t.Errorf("missing key = %q", got)
	}
	if len(*log) != 0 {
		t.Errorf("reported outside dev mode: %q", *log)
	}
	t.Setenv("TELA_DEV", "1")
	T("nope")
	T("nope")
	T("nope2")
	if len(*log) != 2 || !strings.Contains((*log)[0], `no message "nope" for locale es`) || !strings.Contains((*log)[0], "searched es, en") {
		t.Errorf("dev log = %q", *log)
	}
}

func TestMissingKeyWithoutCatalogs(t *testing.T) {
	log := isolate(t)
	t.Setenv("TELA_DEV", "1")
	if got := T("x"); got != "x" {
		t.Errorf("T = %q", got)
	}
	if len(*log) != 1 || !strings.Contains((*log)[0], "call i18n.Load first") {
		t.Errorf("dev log = %q", *log)
	}
}

func TestOddArguments(t *testing.T) {
	log := isolate(t)
	t.Setenv("TELA_DEV", "1")
	mustLoad(t, map[string]string{"en.json": `{"k": "Hi {name}"}`})
	if got := T("k", "name"); got != "Hi {name}" {
		t.Errorf("odd args = %q", got)
	}
	if len(*log) < 1 || !strings.Contains((*log)[0], "odd number of arguments") {
		t.Errorf("dev log = %q", *log)
	}
}

func TestLoadErrors(t *testing.T) {
	isolate(t)
	tests := []struct {
		name  string
		files map[string]string
		glob  string
		want  []string
	}{
		{"no files", nil, "locales/*.json", []string{`matches no files`, "//go:embed"}},
		{"bad pattern", map[string]string{"en.json": "{}"}, "locales/[", []string{"bad pattern"}},
		{"file name", map[string]string{"messages.json": "{}"}, "locales/*.json", []string{"locales/messages.json", "the file name must be a locale"}},
		{"bad json", map[string]string{"en.json": `{"a": `}, "locales/*.json", []string{"locales/en.json", "JSON object"}},
		{"not a string", map[string]string{"en.json": `{"a": 3}`}, "locales/*.json", []string{`message "a" must be a string`}},
		{"icu", map[string]string{"en.json": `{"files": "{count, plural, one {# file}}"}`}, "locales/*.json", []string{"locales/en.json", `message "files"`, `needs an "other" case`}},
		{"nested key", map[string]string{"en.json": `{"nav": {"x": "{a"}}`}, "locales/*.json", []string{`message "nav.x"`, "never closed"}},
	}
	for _, tc := range tests {
		err := Load(catalogs(tc.files), tc.glob)
		if err == nil {
			t.Errorf("%s: no error", tc.name)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: error %q does not contain %q", tc.name, err, w)
			}
		}
	}
	if len(Locales()) != 0 {
		t.Errorf("a failed Load registered %v", Locales())
	}
	// One bad file keeps the good ones out too.
	err := Load(catalogs(map[string]string{"en.json": `{"a": "ok"}`, "es.json": `{"a": "{"}`}), "locales/*.json")
	if err == nil || len(Locales()) != 0 {
		t.Errorf("partial load: err=%v locales=%v", err, Locales())
	}
}

func TestLoadAccumulates(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"en.json": `{"a": "A1", "b": "B1"}`})
	fsys := fstest.MapFS{
		"more/en.json": {Data: []byte(`{"b": "B2", "c": "C2"}`)},
		"more/es.json": {Data: []byte(`{"a": "A-es"}`)},
	}
	if err := Load(fsys, "more/*.json"); err != nil {
		t.Fatal(err)
	}
	if got := T("a") + T("b") + T("c"); got != "A1B2C2" {
		t.Errorf("merged en = %q", got)
	}
	if got := Locales(); len(got) != 2 || got[0] != "en" || got[1] != "es" {
		t.Errorf("Locales = %v", got)
	}
	SetLocale("es")
	if got := T("a"); got != "A-es" {
		t.Errorf("es = %q", got)
	}
}

func TestSystemLocale(t *testing.T) {
	env := map[string]string{}
	setup := func(catalogsLoaded bool) {
		isolate(t)
		getenv = func(k string) string { return env[k] }
		if catalogsLoaded {
			mustLoad(t, enES)
		} else {
			update(func(*state) error { return nil })
		}
	}
	tests := []struct {
		name string
		env  map[string]string
		os   string
		cats bool
		want string
	}{
		{"LANG covered by language", map[string]string{"LANG": "es_MX.UTF-8"}, "", true, "es-MX"},
		{"LANG exact", map[string]string{"LANG": "es_ES.UTF-8@euro"}, "", true, "es-ES"},
		{"LC_ALL wins", map[string]string{"LC_ALL": "en_GB.UTF-8", "LANG": "es_ES.UTF-8"}, "", true, "en-GB"},
		{"LC_MESSAGES over LANG", map[string]string{"LC_MESSAGES": "es", "LANG": "en_US.UTF-8"}, "", true, "es"},
		{"not covered: fallback", map[string]string{"LANG": "fr_FR.UTF-8"}, "", true, "en"},
		{"C: fallback", map[string]string{"LANG": "C"}, "", true, "en"},
		{"POSIX: fallback", map[string]string{"LC_ALL": "POSIX", "LANG": "es"}, "", true, "en"},
		{"unset: fallback", nil, "", true, "en"},
		{"no catalogs: system", map[string]string{"LANG": "fr_FR.UTF-8"}, "", false, "fr-FR"},
		{"no catalogs, no system", nil, "", false, "en"},
		{"OS locale (Windows)", nil, "es-MX", true, "es-MX"},
		{"OS locale not covered", map[string]string{"LANG": "es"}, "ja-JP", true, "en"},
		{"garbage", map[string]string{"LANG": "!!!"}, "", true, "en"},
	}
	for _, tc := range tests {
		env = tc.env
		if env == nil {
			env = map[string]string{}
		}
		setup(tc.cats)
		osLocale = func() string { return tc.os }
		update(func(*state) error { return nil })
		if got := Locale(); got != tc.want {
			t.Errorf("%s: Locale() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSetLocale(t *testing.T) {
	isolate(t)
	mustLoad(t, enES)
	SetLocale("es_MX")
	if got := Locale(); got != "es-MX" {
		t.Errorf("Locale = %q", got)
	}
	// A locale without a catalog is kept for formatting.
	SetLocale("fr")
	if got := Locale(); got != "fr" {
		t.Errorf("Locale = %q", got)
	}
	if got := FormatNumber(1234.5); got != "1 234,5" {
		t.Errorf("FormatNumber = %q", got)
	}
	// An empty tag follows the system again (here: the fallback).
	SetLocale("")
	if got := Locale(); got != "en" {
		t.Errorf("Locale after reset = %q", got)
	}
	defer func() {
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, `SetLocale("nope!")`) || !strings.Contains(s, "BCP 47") {
			t.Errorf("panic = %v", r)
		}
	}()
	SetLocale("nope!")
}

func TestDefaultFallback(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"fr.json": `{"a": "fr a"}`, "es.json": `{"a": "es a"}`})
	// No English: the first catalog loaded (file name order).
	SetLocale("de")
	if got := T("a"); got != "es a" {
		t.Errorf("fallback without en = %q", got)
	}
	// English, once loaded, becomes the fallback.
	mustLoad(t, map[string]string{"en.json": `{"a": "en a"}`})
	if got := T("a"); got != "en a" {
		t.Errorf("fallback with en = %q", got)
	}
}

func TestSetFallbackErrors(t *testing.T) {
	isolate(t)
	mustLoad(t, enES)
	if err := SetFallback("fr"); err == nil || !strings.Contains(err.Error(), "no catalog for \"fr\" is loaded (loaded: en, es)") {
		t.Errorf("SetFallback(fr) = %v", err)
	}
	if err := SetFallback("!!"); err == nil || !strings.Contains(err.Error(), "BCP 47") {
		t.Errorf("SetFallback(!!) = %v", err)
	}
}

func TestFormatFunctionsUseCurrentLocale(t *testing.T) {
	isolate(t)
	SetLocale("de")
	if got := FormatNumber(1234.5); got != "1.234,5" {
		t.Errorf("FormatNumber = %q", got)
	}
	if got := FormatList([]string{"a", "b"}); got != "a und b" {
		t.Errorf("FormatList = %q", got)
	}
	SetLocale("ja")
	if got := FormatList([]string{"a", "b"}); got != "a、b" {
		t.Errorf("FormatList ja = %q", got)
	}
}

// Subscribers hear about locale changes, and only real ones.
func TestSubscribe(t *testing.T) {
	isolate(t)
	mustLoad(t, enES)
	var mu sync.Mutex
	calls := 0
	unsub := subscribe(func() { mu.Lock(); calls++; mu.Unlock() })
	SetLocale("es")
	SetLocale("es") // no change
	SetLocale("en")
	mu.Lock()
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	mu.Unlock()
	unsub()
	SetLocale("es")
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("called after unsubscribe: %d", calls)
	}
}

// SetLocale and T from many goroutines (run with -race).
func TestConcurrentUse(t *testing.T) {
	isolate(t)
	mustLoad(t, enES)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if (i+j)%2 == 0 {
					SetLocale("es")
				} else {
					SetLocale("en")
				}
				if got := T("files", "count", j); got != "" && !strings.Contains(got, "file") && !strings.Contains(got, "archivo") {
					t.Errorf("T = %q", got)
					return
				}
				_ = FormatNumber(float64(j))
				_ = Locale()
			}
		}()
	}
	wg.Wait()
}
