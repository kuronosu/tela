package plist

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// golden is what Marshal writes for sample, byte for byte.
const golden = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Notes &amp; Todos &lt;beta&gt;</string>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key>
			<string>dev.telaui.notes</string>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>notes</string>
			</array>
		</dict>
	</array>
	<key>Empty</key>
	<dict/>
	<key>EmptyArray</key>
	<array/>
	<key>Flag</key>
	<true/>
	<key>Float</key>
	<real>1.5</real>
	<key>Off</key>
	<false/>
	<key>Raw</key>
	<data>
	AAECAwQ=
	</data>
	<key>Size</key>
	<integer>-42</integer>
	<key>When</key>
	<date>2026-11-02T08:30:00Z</date>
</dict>
</plist>
`

func sample() Dict {
	return Dict{
		"CFBundleName": "Notes & Todos <beta>",
		"CFBundleURLTypes": Array{
			Dict{
				"CFBundleURLName":    "dev.telaui.notes",
				"CFBundleURLSchemes": []string{"notes"},
			},
		},
		"Empty":      Dict{},
		"EmptyArray": Array{},
		"Float":      1.5,
		"Flag":       true,
		"Off":        false,
		"Raw":        []byte{0, 1, 2, 3, 4},
		"Size":       -42,
		"When":       time.Date(2026, 11, 2, 9, 30, 0, 0, time.FixedZone("x", 3600)),
	}
}

func TestMarshalGolden(t *testing.T) {
	got, err := Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != golden {
		t.Errorf("Marshal printed\n%s\nwant\n%s", got, golden)
	}
}

func TestRoundTrip(t *testing.T) {
	out, err := Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	v, err := Unmarshal(out)
	if err != nil {
		t.Fatal(err)
	}
	want := Dict{
		"CFBundleName": "Notes & Todos <beta>",
		"CFBundleURLTypes": Array{
			Dict{
				"CFBundleURLName":    "dev.telaui.notes",
				"CFBundleURLSchemes": Array{"notes"},
			},
		},
		"Empty":      Dict{},
		"EmptyArray": Array{},
		"Float":      1.5,
		"Flag":       true,
		"Off":        false,
		"Raw":        []byte{0, 1, 2, 3, 4},
		"Size":       int64(-42),
		"When":       time.Date(2026, 11, 2, 8, 30, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("decoded\n%#v\nwant\n%#v", v, want)
	}
	// Decoding then encoding is a fixed point: an edit rewrites the same bytes.
	again, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, out) {
		t.Errorf("re-encoding changed the document:\n%s\nwas\n%s", again, out)
	}
}

func TestUnmarshalKeepsUnknownKeys(t *testing.T) {
	// A document written by another tool: another key order, a comment, a
	// key and elements this package never writes, spaces and CRLF.
	src := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n" +
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\r\n" +
		`<plist version="1.0"><!-- made by hand -->
  <dict>
    <key>Zeta</key><string>last</string>
    <key>LSUIElement</key> <true/>
    <key>CFBundleDocumentTypes</key>
    <array>
      <dict><key>CFBundleTypeExtensions</key><array><string>md</string><string>txt</string></array>
      <key>CFBundleTypeRole</key><string>Editor</string></dict>
    </array>
    <key>Big</key><integer>9223372036854775807</integer>
    <key>Real</key><real>2e3</real>
    <key>Blob</key><data>
      AAEC
      AwQ=
    </data>
  </dict>
</plist>`
	d, err := UnmarshalDict([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	d["CFBundleName"] = "X"
	out, err := Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalDict(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Zeta", "LSUIElement", "CFBundleDocumentTypes", "Big", "Real", "Blob", "CFBundleName"} {
		if _, ok := back[k]; !ok {
			t.Errorf("key %s lost", k)
		}
	}
	if back["Big"] != int64(math.MaxInt64) || back["Real"] != 2000.0 || !bytes.Equal(back["Blob"].([]byte), []byte{0, 1, 2, 3, 4}) {
		t.Errorf("values changed: %#v", back)
	}
	types := back["CFBundleDocumentTypes"].(Array)[0].(Dict)
	if !reflect.DeepEqual(types["CFBundleTypeExtensions"], Array{"md", "txt"}) {
		t.Errorf("nested array: %#v", types)
	}
	if i, z := strings.Index(string(out), "<key>Big"), strings.Index(string(out), "<key>Zeta"); i < 0 || z < 0 || i > z {
		t.Errorf("keys are not sorted:\n%s", out)
	}
}

func TestMarshalErrors(t *testing.T) {
	type s struct{}
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"nil", nil, "nil cannot be encoded"},
		{"nil value", Dict{"A": nil}, "root.A: nil cannot be encoded"},
		{"struct", Dict{"A": Array{s{}}}, "root.A[0]: a plist.s cannot be encoded"},
		{"control character", Dict{"A": "x\x01"}, "root.A: the character U+0001 cannot appear in XML"},
		{"invalid UTF-8", Dict{"A": "\xff"}, "not valid UTF-8"},
		{"big unsigned", Dict{"A": uint64(math.MaxUint64)}, "above the int64 range"},
		{"int keys", map[int]string{1: "a"}, "string keys"},
	} {
		_, err := Marshal(tc.v)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}

func TestUnmarshalErrors(t *testing.T) {
	wrap := func(body string) string { return `<plist version="1.0">` + body + `</plist>` }
	for _, tc := range []struct {
		name, src, want string
	}{
		{"empty", "", "no <plist>"},
		{"binary", "bplist00\x00", "binary property list"},
		{"wrong root", "<html></html>", "want <plist>"},
		{"no value", wrap(""), "holds no value"},
		{"two values", wrap("<string>a</string><string>b</string>"), "more than one value"},
		{"unknown element", wrap("<blob/>"), "unknown element <blob>"},
		{"key without value", wrap("<dict><key>A</key></dict>"), `"A" has no value`},
		{"value without key", wrap("<dict><string>a</string></dict>"), "found <string> where a key goes"},
		{"duplicate key", wrap("<dict><key>A</key><true/><key>A</key><true/></dict>"), `duplicate key "A"`},
		{"bad integer", wrap("<integer>abc</integer>"), "not an integer"},
		{"bad real", wrap("<real>x</real>"), "not a number"},
		{"bad date", wrap("<date>yesterday</date>"), "not a date"},
		{"bad data", wrap("<data>!!</data>"), "not valid base64"},
		{"nested text", wrap("<string><b/></string>"), "takes text only"},
		{"stray text", wrap("<dict>oops</dict>"), "unexpected text"},
		{"truncated", `<plist version="1.0"><dict><key>A`, "plist:"},
		{"deep", wrap(strings.Repeat("<array>", maxDepth+5) + strings.Repeat("</array>", maxDepth+5)), "nesting deeper"},
	} {
		_, err := Unmarshal([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	if _, err := UnmarshalDict([]byte(wrap("<array/>"))); err == nil || !strings.Contains(err.Error(), "root is a array") {
		t.Errorf("UnmarshalDict of an array: %v", err)
	}
}

func TestDataWraps(t *testing.T) {
	data := bytes.Repeat([]byte{0xAB}, 100)
	out, err := Marshal(Dict{"D": data})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines[4:] { // after the header
		if len(strings.TrimLeft(l, "\t")) > dataLine+10 {
			t.Errorf("line longer than the wrap: %q", l)
		}
	}
	back, err := UnmarshalDict(out)
	if err != nil || !bytes.Equal(back["D"].([]byte), data) {
		t.Errorf("round trip of wrapped data: %v", err)
	}
	if out, _ := Marshal(Dict{"D": []byte{}}); !strings.Contains(string(out), "<data></data>") {
		t.Errorf("empty data: %s", out)
	}
}

func TestSpecialReals(t *testing.T) {
	out, err := Marshal(Array{math.Inf(1), math.Inf(-1), math.NaN(), 0.1, float32(0.5)})
	if err != nil {
		t.Fatal(err)
	}
	v, err := Unmarshal(out)
	if err != nil {
		t.Fatal(err)
	}
	a := v.(Array)
	if !math.IsInf(a[0].(float64), 1) || !math.IsInf(a[1].(float64), -1) || !math.IsNaN(a[2].(float64)) || a[3] != 0.1 || a[4] != 0.5 {
		t.Errorf("reals: %v", a)
	}
}

// Integers read as Apple's parser reads them: decimal (a leading zero too)
// or 0x hexadecimal, never octal, binary or with underscores.
func TestIntegers(t *testing.T) {
	for in, want := range map[string]int64{"010": 10, "+7": 7, "-12": -12, "0x1F": 31, "0X1f": 31, "-0x10": -16, " 42 ": 42} {
		v, err := Unmarshal([]byte("<plist version=\"1.0\"><integer>" + in + "</integer></plist>"))
		if err != nil || v != want {
			t.Errorf("<integer>%s</integer> = %v, %v; want %d", in, v, err, want)
		}
	}
	for _, in := range []string{"1_000", "0o7", "0b1", "0x", "--1", "-+1", "0x-1", ""} {
		if v, err := Unmarshal([]byte("<plist version=\"1.0\"><integer>" + in + "</integer></plist>")); err == nil {
			t.Errorf("<integer>%s</integer> = %v, want an error", in, v)
		}
	}
}

// A carriage return survives an encode and decode (XML would turn a raw
// one into a line feed).
func TestCarriageReturnRoundTrips(t *testing.T) {
	in := Dict{"K": "a\r\nb\rc"}
	out, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Unmarshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.(Dict)["K"]; got != "a\r\nb\rc" {
		t.Errorf("round trip gave %q, want %q\n%s", got, "a\r\nb\rc", out)
	}
	again, _ := Marshal(v)
	if string(again) != string(out) {
		t.Errorf("the second encoding differs:\n%s\n%s", out, again)
	}
}
