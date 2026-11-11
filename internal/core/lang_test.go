package core

import (
	"strings"
	"testing"
)

func TestValidLangTag(t *testing.T) {
	for _, tag := range []string{
		"en", "ja", "zh-Hans", "zh-Hant-TW", "zh-HK", "pt-BR", "es-419", "sr-Latn-RS", "de-CH-1996",
		"sl-rozaj-biske", "en-US-u-ca-gregory", "ar-EG-u-nu-arab", "x-private", "en-x-tela", "zh-min-nan",
		"i-klingon", "EN-us", "yue-Hant-HK", "haw", "tlh",
	} {
		if !validLangTag(tag) {
			t.Errorf("%q rejected", tag)
		}
	}
	for _, tag := range []string{
		"", "e", "es_MX", "en-", "-en", "en--US", "toolongtag1", "en-US-u", "en-x", "1en", "zh-Hans-Hant-TW",
		"en US", "日本", "en-a-bb-a", "x", "en-Latn-US-x", "toolong-x",
	} {
		if validLangTag(tag) {
			t.Errorf("%q accepted", tag)
		}
	}
	// A 4 to 8 letter language subtag is well formed (reserved or
	// registered), as RFC 5646 says.
	if !validLangTag("english") {
		t.Error(`"english" rejected: a 5-8 letter language subtag is well formed`)
	}
}

func TestLangDevPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Lang(es_MX) did not panic in dev mode")
		}
		if msg := r.(string); !strings.Contains(msg, `did you mean "es-MX"`) {
			t.Errorf("message %q", msg)
		}
	}()
	Lang("es_MX")
}

func TestCheckLang(t *testing.T) {
	if CheckLang("ja") != "" || CheckLang("") != "" {
		t.Error("valid tags reported")
	}
	if !strings.Contains(CheckLang("pt_BR"), "BCP 47") {
		t.Errorf("message %q", CheckLang("pt_BR"))
	}
}

// paraLang returns the language of the default style of the paragraph of
// the element with key k, and that of its spans.
func paraLang(t *testing.T, tr *Tree, k string) (string, []string) {
	t.Helper()
	el := keyed(tr, k)
	if el == nil || el.box.para == nil {
		t.Fatalf("%s has no paragraph", k)
	}
	var spans []string
	for _, s := range el.box.para.spans {
		spans = append(spans, s.Style.Language)
	}
	return el.box.para.def.Language, spans
}

func TestLangInheritedAndSwitched(t *testing.T) {
	var setLang Setter[string]
	app := Component(func() Node {
		l, set := UseState("ja")
		setLang = set
		return div(Key("root"), Lang(l),
			p(Key("p"), Text("直 "), span(Lang("zh-Hans"), Text("直")), span(Text(" 骨"))),
			div(Key("ko"), Lang("ko"), p(Key("q"), Text("骨"))),
		)
	})
	tr := newTestTree(t, 400, 200, app, nil)
	tr.Frame()
	def, spans := paraLang(t, tr, "p")
	if def != "ja" || len(spans) != 2 || spans[0] != "zh-Hans" || spans[1] != "ja" {
		t.Fatalf("paragraph %q, spans %q", def, spans)
	}
	if def, _ := paraLang(t, tr, "q"); def != "ko" {
		t.Errorf("Lang(ko) inside ja: %q", def)
	}
	setLang.Set("zh-TW")
	tr.Frame()
	if def, spans := paraLang(t, tr, "p"); def != "zh-TW" || spans[1] != "zh-TW" || spans[0] != "zh-Hans" {
		t.Errorf("after the switch: paragraph %q, spans %q", def, spans)
	}
	if def, _ := paraLang(t, tr, "q"); def != "ko" {
		t.Errorf("Lang(ko) after the switch: %q", def)
	}
	setLang.Set("")
	tr.Frame()
	if def, _ := paraLang(t, tr, "p"); def != DefaultLang() {
		t.Errorf("Lang(\"\") gives %q, want the default %q", def, DefaultLang())
	}
}

func TestLangProvider(t *testing.T) {
	var setLang Setter[string]
	inner := Component(func() Node { return p(Key("p"), Text("直")) })
	app := Component(func() Node {
		l, set := UseState("ja")
		setLang = set
		return div(Key("root"), LangProvider(l, Fragment(inner, p(Key("own"), Lang("ko"), Text("骨")))))
	})
	tr := newTestTree(t, 400, 200, app, nil)
	tr.Frame()
	if def, _ := paraLang(t, tr, "p"); def != "ja" {
		t.Fatalf("below LangProvider(ja): %q", def)
	}
	if def, _ := paraLang(t, tr, "own"); def != "ko" {
		t.Errorf("own Lang below a provider: %q", def)
	}
	setLang.Set("zh-Hans")
	tr.Frame()
	if def, _ := paraLang(t, tr, "p"); def != "zh-Hans" {
		t.Errorf("after the provider's change: %q", def)
	}
	if lang := keyed(tr, "root").box.cs.Text.Language; lang != DefaultLang() {
		t.Errorf("the provider's parent has %q", lang)
	}
}

func TestDefaultLang(t *testing.T) {
	// A test binary links the deterministic fonts: the system's
	// language is ignored.
	if got := DefaultLang(); got != "" {
		t.Fatalf("default language in a test binary: %q", got)
	}
	tr := newTestTree(t, 400, 200, div(Key("root"), p(Key("p"), Text("x")), p(Key("own"), Lang("ja"), Text("x"))), nil)
	tr.Frame()
	SetDefaultLang("ar")
	defer func() {
		SetDefaultLang("")
		if DefaultLang() != "" {
			t.Error("SetDefaultLang(\"\") did not go back to the system's")
		}
	}()
	if DefaultLang() != "ar" {
		t.Fatalf("DefaultLang %q", DefaultLang())
	}
	tr.syncLang() // what the posted update does on the UI thread
	tr.Frame()
	if def, _ := paraLang(t, tr, "p"); def != "ar" {
		t.Errorf("default language not applied: %q", def)
	}
	if def, _ := paraLang(t, tr, "own"); def != "ja" {
		t.Errorf("own Lang under the default: %q", def)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("SetDefaultLang(en_US) did not panic")
			}
		}()
		SetDefaultLang("en_US")
	}()
}
