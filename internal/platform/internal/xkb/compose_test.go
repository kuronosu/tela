package xkb

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureComposeOptions(t testing.TB) ComposeOptions {
	t.Helper()
	dir, err := filepath.Abs("testdata/compose/locale")
	if err != nil {
		t.Fatal(err)
	}
	return ComposeOptions{Locale: "en_US.UTF-8", Home: "/nonexistent-home", LocaleDir: dir}
}

func noEnv(string) string { return "" }

// TestComposeAgainstLibxkbcommon feeds the sequences of
// testdata/compose/sequences.txt to the fixture tables and compares each
// status and the final result with libxkbcommon's (the .expected files,
// from the oracle's compose mode). Where libxkbcommon reports a
// cancelled sequence, ComposeState feeds the keysym again from the start
// (see Feed), so those steps are compared with a fresh state instead.
func TestComposeAgainstLibxkbcommon(t *testing.T) {
	o := fixtureComposeOptions(t)
	for _, tc := range []struct{ file, expected string }{
		{"testdata/compose/locale/en_US.UTF-8/Compose", "testdata/compose/locale.expected"},
		{"testdata/compose/user.XCompose", "testdata/compose/user.expected"},
	} {
		data, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		table, err := ParseCompose(data, tc.file, o)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		exp, err := os.ReadFile(tc.expected)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(exp)), "\n") {
			seqPart, resPart, _ := strings.Cut(line, " => ")
			names := strings.Fields(seqPart)
			res := strings.Fields(resPart)
			statuses, wantHex := res[:len(res)-1], res[len(res)-1]
			st := NewComposeState(table)
			var text string
			cancelled := false
			for i, name := range names {
				ks, ok := KeysymFromName(name)
				if !ok {
					t.Fatalf("unknown keysym %q", name)
				}
				got, out := st.Feed(ks)
				want := statuses[i]
				if want == "cancelled" {
					cancelled = true
					fresh, _ := NewComposeState(table).Feed(ks)
					if got != fresh {
						t.Errorf("%s: %q after a cancelled sequence: %v, want %v (fed from the start)", tc.file, seqPart, got, fresh)
					}
					continue
				}
				if cancelled {
					continue // libxkbcommon's later steps follow its own cancel
				}
				if got.String() != want {
					t.Errorf("%s: %q step %d (%s): %v, want %s", tc.file, seqPart, i, name, got, want)
				}
				text = out // libxkbcommon reports the result of the last step
			}
			if cancelled {
				continue
			}
			wantText := ""
			if wantHex != "-" && statuses[len(statuses)-1] == "composed" {
				b, _ := hex.DecodeString(wantHex)
				wantText = string(b)
			}
			if text != wantText {
				t.Errorf("%s: %q composed %q, want %q", tc.file, seqPart, text, wantText)
			}
		}
	}
}

func (s ComposeStatus) String() string {
	switch s {
	case ComposeNothing:
		return "nothing"
	case ComposeComposing:
		return "composing"
	case ComposeComposed:
		return "composed"
	case ComposeIgnored:
		return "ignored"
	}
	return "invalid"
}

func TestComposeCancelRefeeds(t *testing.T) {
	data, _ := os.ReadFile("testdata/compose/locale/en_US.UTF-8/Compose")
	table, err := ParseCompose(data, "Compose", fixtureComposeOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	st := NewComposeState(table)
	ks := func(n string) Keysym { k, _ := KeysymFromName(n); return k }
	steps := []struct {
		sym    string
		status ComposeStatus
		text   string
	}{
		{"dead_acute", ComposeComposing, ""},
		{"q", ComposeNothing, ""}, // cancels; q types normally
		{"dead_acute", ComposeComposing, ""},
		{"dead_grave", ComposeComposing, ""}, // cancels and starts a new sequence
		{"a", ComposeComposed, "à"},
		{"Multi_key", ComposeComposing, ""},
		{"Shift_L", ComposeIgnored, ""},
		{"quotedbl", ComposeComposing, ""},
		{"a", ComposeComposed, "ä"},
	}
	for i, s := range steps {
		got, text := st.Feed(ks(s.sym))
		if got != s.status || text != s.text {
			t.Errorf("step %d (%s): %v %q, want %v %q", i, s.sym, got, text, s.status, s.text)
		}
	}
	st.Feed(ks("dead_acute"))
	if !st.Composing() {
		t.Error("not composing after a dead key")
	}
	st.Reset()
	if got, _ := st.Feed(ks("e")); got != ComposeNothing {
		t.Errorf("after Reset, e = %v", got)
	}
	var nilState *ComposeState
	if got, _ := nilState.Feed(ks("e")); got != ComposeNothing {
		t.Errorf("nil state Feed = %v", got)
	}
}

func TestLoadComposeTableOrder(t *testing.T) {
	o := fixtureComposeOptions(t)
	user, _ := filepath.Abs("testdata/compose/user.XCompose")
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	// The system file of the locale, after locale.alias.
	for _, locale := range []string{"en_US.UTF-8", "en_US.utf8", "C.UTF-8", "es_CO.UTF-8", "de_DE.UTF-8", "C", "POSIX"} {
		o.Locale = locale
		_, path, err := LoadComposeTable(noEnv, o)
		if err != nil || !strings.HasSuffix(path, "en_US.UTF-8/Compose") {
			t.Errorf("locale %s: path %q, err %v", locale, path, err)
		}
	}
	o.Locale = "xx_XX.UTF-8"
	if _, _, err := LoadComposeTable(noEnv, o); !errors.Is(err, ErrNoComposeFile) {
		t.Errorf("unknown locale: err %v, want ErrNoComposeFile", err)
	}

	// XCOMPOSEFILE wins, and its include "%L" pulls in the locale file.
	o.Locale = "en_US.UTF-8"
	table, path, err := LoadComposeTable(env(map[string]string{"XCOMPOSEFILE": user}), o)
	if err != nil || path != user {
		t.Fatalf("XCOMPOSEFILE: path %q, err %v", path, err)
	}
	st := NewComposeState(table)
	st.Feed(0xff20) // Multi_key
	st.Feed('h')
	if s, text := st.Feed('i'); s != ComposeComposed || text != "hello" {
		t.Errorf("user sequence: %v %q", s, text)
	}
	st.Feed(0xfe51) // dead_acute
	if s, text := st.Feed(' '); s != ComposeComposed || text != "'" {
		t.Errorf("included sequence: %v %q", s, text)
	}

	// ~/.XCompose and $XDG_CONFIG_HOME/XCompose are read through
	// ReadFile; a missing XCOMPOSEFILE falls through to them.
	var tried []string
	o.ReadFile = func(p string) ([]byte, error) {
		tried = append(tried, p)
		return os.ReadFile(p)
	}
	o.Home = "/home/u"
	LoadComposeTable(env(map[string]string{"XCOMPOSEFILE": "/missing"}), o)
	if len(tried) < 3 || tried[0] != "/missing" || tried[1] != "/home/u/.config/XCompose" || tried[2] != "/home/u/.XCompose" {
		t.Errorf("lookup order %v", tried)
	}
	tried = nil
	LoadComposeTable(env(map[string]string{"XDG_CONFIG_HOME": "/xdg"}), o)
	if len(tried) < 2 || tried[0] != "/xdg/XCompose" || tried[1] != "/home/u/.XCompose" {
		t.Errorf("lookup order with XDG_CONFIG_HOME %v", tried)
	}
}

func TestComposeErrors(t *testing.T) {
	o := fixtureComposeOptions(t)
	for _, tc := range []struct{ src, want string }{
		{`<a> <b> "x"`, "missing ':'"},
		{`<a : "x"`, "unterminated"},
		{`include "%Q"`, "unknown %Q"},
		{`include "/nonexistent/Compose"`, "cannot read include"},
	} {
		_, err := ParseCompose([]byte(tc.src), "test", o)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: err %v, want %q", tc.src, err, tc.want)
		}
	}
	// Unknown keysyms skip the line.
	if _, err := ParseCompose([]byte("<nosuchkeysym> <a> : \"x\"\n"), "test", o); err != nil {
		t.Errorf("unknown keysym: %v", err)
	}
}

// TestSystemCompose reads the real Compose file of this machine. It reads
// system files, so it only runs with TELA_SYSTEM_COMPOSE=1.
func TestSystemCompose(t *testing.T) {
	if os.Getenv("TELA_SYSTEM_COMPOSE") == "" {
		t.Skip("set TELA_SYSTEM_COMPOSE=1 to parse this machine's Compose file")
	}
	o := ComposeOptionsFromEnv(os.Getenv)
	start := time.Now()
	table, path, err := LoadComposeTable(os.Getenv, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d nodes, parsed in %v", path, len(table.nodes), time.Since(start))
	o.Locale = "en_US.UTF-8"
	sys, ok := o.localeComposePath(o.Locale)
	if !ok {
		t.Skip("no system en_US.UTF-8 Compose file")
	}
	data, err := os.ReadFile(sys)
	if err != nil {
		t.Skip(err)
	}
	start = time.Now()
	table, err = ParseCompose(data, sys, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d nodes, parsed in %v", sys, len(table.nodes), time.Since(start))
	st := NewComposeState(table)
	st.Feed(0xfe51) // dead_acute
	if s, text := st.Feed('e'); s != ComposeComposed || text != "é" {
		t.Errorf("dead_acute e: %v %q", s, text)
	}
}
