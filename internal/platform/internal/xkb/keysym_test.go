package xkb

import (
	"bufio"
	"compress/gzip"
	"os"
	"strconv"
	"strings"
	"testing"
)

// readGzipLines returns the lines of a gzip-compressed testdata file.
func readGzipLines(t testing.TB, path string) []string {
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
	var lines []string
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func parseHex(t testing.TB, s string) uint32 {
	t.Helper()
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 32)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return uint32(v)
}

// TestKeysymsAgainstLibxkbcommon compares ToUTF32, ToUpper and ToLower
// with libxkbcommon 1.13.2 (testdata/syms.txt.gz, from the oracle's syms
// mode) for every keysym of xkbcommon-keysyms.h and every value below
// 0x10000: the file lists those with a character or a case mapping, every
// other value must have neither.
func TestKeysymsAgainstLibxkbcommon(t *testing.T) {
	type want struct{ utf, upper, lower uint32 }
	known := map[Keysym]want{}
	for _, l := range readGzipLines(t, "testdata/syms.txt.gz") {
		f := strings.Fields(l)
		known[Keysym(parseHex(t, f[0]))] = want{parseHex(t, f[1]), parseHex(t, f[2]), parseHex(t, f[3])}
	}
	check := func(ks Keysym, w want) {
		t.Helper()
		if got := uint32(ks.ToUTF32()); got != w.utf {
			t.Errorf("%v (%#x).ToUTF32() = %#x, want %#x", ks, uint32(ks), got, w.utf)
		}
		if got := uint32(ks.ToUpper()); got != w.upper {
			t.Errorf("%v (%#x).ToUpper() = %#x, want %#x", ks, uint32(ks), got, w.upper)
		}
		if got := uint32(ks.ToLower()); got != w.lower {
			t.Errorf("%v (%#x).ToLower() = %#x, want %#x", ks, uint32(ks), got, w.lower)
		}
	}
	for ks, w := range known {
		check(ks, w)
	}
	for v := uint32(0); v < 0x10000; v++ {
		if _, ok := known[Keysym(v)]; !ok {
			check(Keysym(v), want{0, v, v})
		}
	}
	for _, n := range keysymNames {
		if _, ok := known[n.sym]; !ok {
			check(n.sym, want{0, uint32(n.sym), uint32(n.sym)})
		}
	}
}

func TestKeysymFromName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want Keysym
		ok   bool
	}{
		{"a", 0x61, true},
		{"A", 0x41, true},
		{"1", 0x31, true},
		{"Return", 0xff0d, true},
		{"dead_acute", 0xfe51, true},
		{"dead_perispomeni", 0xfe53, true}, // alias of dead_tilde
		{"Cyrillic_ie", 0x6c5, true},
		{"XF86AudioMute", 0x1008ff12, true},
		{"XF86_AudioMute", 0x1008ff12, true},
		{"U20AC", 0x10020ac, true},
		{"U00E9", 0xe9, true}, // Latin-1 normalizes to the legacy keysym
		{"U0041", 0x41, true},
		{"0x1000e9", 0x1000e9, true},
		{"NoSymbol", 0, true},
		{"nosuchkey", 0, false},
		{"U", 0x55, true}, // a name, not a Unicode form
		{"UZZ", 0, false},
	} {
		got, ok := KeysymFromName(tc.name)
		if got != tc.want || ok != tc.ok {
			t.Errorf("KeysymFromName(%q) = %#x, %v; want %#x, %v", tc.name, uint32(got), ok, uint32(tc.want), tc.ok)
		}
	}
	if s := Keysym(0xfe53).String(); s != "dead_tilde" {
		t.Errorf("String of 0xfe53 = %q, want the canonical dead_tilde", s)
	}
	if s := Keysym(0x10020ac).String(); s != "EuroSign" && s != "U20AC" {
		t.Errorf("String of U+20AC keysym = %q", s)
	}
}
