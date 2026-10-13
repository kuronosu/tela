package xkb

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fixtures are the keymaps of testdata/keymaps, dumped with libxkbcommon
// 1.13.2's xkbcli compile-keymap --output-format 1 (what wlroots, Mutter
// and KWin send) from xkeyboard-config; see testdata/README.
var fixtures = []string{
	"us", "us-intl", "us-altgr-intl", "de", "fr", "es", "latam", "gb", "ru", "il", "gr", "jp",
	"us-nocaps", "us-compose-ralt", "us-ru-grp", "de-neo",
}

func readGzip(t testing.TB, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func loadFixture(t testing.TB, name string) *Keymap {
	t.Helper()
	km, err := ParseKeymap(readGzip(t, "testdata/keymaps/"+name+".xkb.gz"))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return km
}

// stripControls removes the control characters libxkbcommon's
// xkb_state_key_get_utf8 returns for Return, Tab, Escape, … (Text never
// has them, ADR-050).
func stripControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// TestKeymapsAgainstLibxkbcommon parses every fixture and compares, with
// the expected results of libxkbcommon (testdata/oracle): the resolved
// modifier masks, which keys repeat, and the keysym and text of every key
// in every listed state, Control added or not.
func TestKeymapsAgainstLibxkbcommon(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			km := loadFixture(t, name)
			lines := strings.Split(string(readGzip(t, "testdata/keymaps/"+name+".oracle.gz")), "\n")
			st := NewState(km)
			type result struct {
				sym  Keysym
				text string
			}
			var (
				label string
				want  map[uint32]result
				dep   uint32
				lock  uint32
				group uint32
				nStat int
			)
			check := func() {
				if want == nil {
					return
				}
				nStat++
				for _, ctrl := range []uint32{0, km.ctrlMask} {
					st.UpdateMask(dep|ctrl, 0, lock, group)
					for kc := km.minKey; kc < km.minKey+uint32(len(km.keys)); kc++ {
						w := want[kc]
						got := st.Lookup(kc)
						if got.Sym != w.sym || got.Text != stripControls(w.text) {
							t.Errorf("state %s (ctrl %v), key %d <%s>: got %v %q, want %v %q",
								label, ctrl != 0, kc, km.keys[kc-km.minKey].name, got.Sym, got.Text, w.sym, stripControls(w.text))
						}
					}
				}
			}
			for _, l := range lines {
				f := strings.Fields(l)
				if len(f) == 0 {
					continue
				}
				switch f[0] {
				case "mask":
					got, ok := km.ModMask(f[1])
					if w := parseHex(t, f[2]); !ok || got != w {
						t.Errorf("mask of %s = %#x (%v), want %#x", f[1], got, ok, w)
					}
				case "layouts":
					if n, _ := strconv.Atoi(f[1]); km.NumGroups() != n {
						t.Errorf("NumGroups = %d, want %d", km.NumGroups(), n)
					}
				case "norepeat":
					var wantNo, gotNo []uint32
					for _, s := range f[1:] {
						v, _ := strconv.Atoi(s)
						wantNo = append(wantNo, uint32(v))
					}
					for i, k := range km.keys {
						if k.exists && len(k.groups) > 0 && !k.repeats {
							gotNo = append(gotNo, km.minKey+uint32(i))
						}
					}
					if !slices.Equal(gotNo, wantNo) {
						t.Errorf("keys that do not repeat: got %v, want %v", gotNo, wantNo)
					}
				case "state":
					check()
					label = f[1]
					dep, lock = parseHex(t, f[2]), parseHex(t, f[3])
					g, _ := strconv.Atoi(f[4])
					group = uint32(g)
					want = map[uint32]result{}
				default:
					kc, _ := strconv.Atoi(f[0])
					r := result{sym: Keysym(parseHex(t, f[1]))}
					if f[2] != "-" {
						b, err := hex.DecodeString(f[2])
						if err != nil {
							t.Fatal(err)
						}
						r.text = string(b)
					}
					want[uint32(kc)] = r
				}
			}
			check()
			if nStat == 0 {
				t.Fatal("no states in the oracle file")
			}
		})
	}
}

// TestParseSpeed checks that parsing a real keymap takes well under the
// 10 ms budget (it runs once per keymap event, on the UI thread).
func TestParseSpeed(t *testing.T) {
	text := readGzip(t, "testdata/keymaps/us-ru-grp.xkb.gz")
	if _, err := ParseKeymap(text); err != nil { // warm the keysym tables
		t.Fatal(err)
	}
	best := time.Hour
	for range 5 {
		start := time.Now()
		if _, err := ParseKeymap(text); err != nil {
			t.Fatal(err)
		}
		best = min(best, time.Since(start))
	}
	t.Logf("ParseKeymap(us,ru): %v", best)
	if best > 10*time.Millisecond && !testing.Short() {
		t.Errorf("ParseKeymap took %v, want under 10 ms", best)
	}
}

func BenchmarkParseKeymap(b *testing.B) {
	text := readGzip(b, "testdata/keymaps/us.xkb.gz")
	b.SetBytes(int64(len(text)))
	for b.Loop() {
		if _, err := ParseKeymap(text); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLookupDoesNotAllocate(t *testing.T) {
	km := loadFixture(t, "de")
	st := NewState(km)
	shift, _ := km.ModMask("Shift")
	lock, _ := km.ModMask("Lock")
	allocs := testing.AllocsPerRun(100, func() {
		for kc := uint32(8); kc < 300; kc++ {
			st.UpdateMask(shift, 0, lock, 0)
			_ = st.Lookup(kc)
			_ = st.Modifiers()
		}
	})
	if allocs != 0 {
		t.Errorf("Lookup allocates %v times per run", allocs)
	}
}

func TestKeymapTrailingNULAndErrors(t *testing.T) {
	text := readGzip(t, "testdata/keymaps/us.xkb.gz")
	if _, err := ParseKeymap(append(bytes.Clone(text), 0, 0, 0)); err != nil {
		t.Errorf("keymap with trailing NULs: %v", err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"include", `xkb_keymap { xkb_keycodes { include "evdev" }; };`, "includes are not supported"},
		{"no symbols", `xkb_keymap { xkb_keycodes { <A> = 10; }; xkb_types { }; };`, "no xkb_symbols"},
		{"unknown key field", `xkb_keymap { xkb_keycodes { <A> = 10; }; xkb_types { type "ONE_LEVEL" { modifiers = none; }; };
			xkb_symbols { key <A> { frobnicate = 3, [ a ] }; }; };`, "unknown field"},
		{"unknown modifier", `xkb_keymap { xkb_keycodes { <A> = 10; }; xkb_types { type "T" { modifiers = Bogus; }; }; xkb_symbols { }; };`, "unknown modifier"},
		{"unterminated", `xkb_keymap { xkb_keycodes { <A> = 10;`, ""},
	} {
		_, err := ParseKeymap([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one containing %q", tc.name, err, tc.want)
		}
	}
}
