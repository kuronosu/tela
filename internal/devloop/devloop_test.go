package devloop

import (
	"errors"
	"go/build"
	"slices"
	"strings"
	"testing"
)

func TestStateReport(t *testing.T) {
	for _, r := range []StateReport{{}, {Kept: 12, Reset: 1}, {Kept: 1 << 20, Reset: 300}} {
		got, err := DecodeStateReport(r.Encode())
		if err != nil || got != r {
			t.Errorf("round trip of %+v: %+v, %v", r, got, err)
		}
	}
	// Trailing bytes are ignored (the report may grow).
	p := append(StateReport{Kept: 3, Reset: 4}.Encode(), 9, 9, 9)
	if got, err := DecodeStateReport(p); err != nil || got != (StateReport{3, 4}) {
		t.Errorf("with trailing bytes: %+v, %v", got, err)
	}
	for _, bad := range [][]byte{nil, {0x80}, {1}, {0xff, 0xff, 0xff, 0xff, 0x0f, 0}} {
		if _, err := DecodeStateReport(bad); err == nil {
			t.Errorf("DecodeStateReport(%v) accepted", bad)
		}
	}
}

func TestAssets(t *testing.T) {
	dirs := []string{"/home/a/assets", "/srv/b c/img", `C:\x\y`}
	if got := DecodeAssets(EncodeAssets(dirs)); !slices.Equal(got, dirs) {
		t.Errorf("round trip: %q", got)
	}
	if got := DecodeAssets(nil); len(got) != 0 {
		t.Errorf("empty payload: %q", got)
	}
	if got := DecodeAssets([]byte("\x00/a\x00\x00")); !slices.Equal(got, []string{"/a"}) {
		t.Errorf("empty paths kept: %q", got)
	}
}

func TestPanic(t *testing.T) {
	p := PanicPayload(errors.New("index out of range"), []byte("goroutine 1 [running]:\nmain.main()\n\n"))
	want := "panic: index out of range\n\ngoroutine 1 [running]:\nmain.main()"
	if string(p) != want {
		t.Errorf("payload %q, want %q", p, want)
	}
	if s := PanicSummary(p); s != "panic: index out of range" {
		t.Errorf("summary %q", s)
	}
}

// TestImportsOnlyStdlib keeps the package importable by the child runtime
// without pulling anything else into teladev builds.
func TestImportsOnlyStdlib(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(strings.SplitN(imp, "/", 2)[0], ".") {
			t.Errorf("devloop imports %s; it may import only the standard library", imp)
		}
	}
	for _, k := range []string{KindStateSave, KindStateLoad, KindStateReport, KindPanic, KindAssets, KindQuit} {
		if !strings.HasPrefix(k, "tela.dev.") {
			t.Errorf("kind %q lacks the tela.dev. prefix", k)
		}
	}
}
