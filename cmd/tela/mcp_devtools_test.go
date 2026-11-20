package main

import (
	"go/parser"
	"go/token"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMCPLogsRecord(t *testing.T) {
	var l mcpLogs
	l.record(mcpReply{
		Logs:   []string{"dev message: unknown class \"foo\"", "dev warning: missing key", "telatest: something", ""},
		Output: "plain line\n2026/11/20 10:00:00 ERROR cannot save\nlevel=WARN msg=slow\n\n",
	})
	got, earlier := l.since(false, "")
	var lines []string
	for _, e := range got {
		lines = append(lines, e.Level+"/"+e.Source+": "+e.Msg)
	}
	want := []string{`error/dev: unknown class "foo"`, "warn/dev: missing key", "info/telatest: something",
		"info/app: plain line", "error/app: 2026/11/20 10:00:00 ERROR cannot save", "warn/app: level=WARN msg=slow"}
	if earlier != 0 || strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("entries\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if got, _ := l.since(false, ""); len(got) != 0 {
		t.Errorf("a second call listed %d entries again", len(got))
	}
	l.record(mcpReply{Logs: []string{"dev warning: another"}})
	if got, _ := l.since(false, "error"); len(got) != 0 {
		t.Errorf("level error listed %v", got)
	}
	if got, _ := l.since(true, "warn"); len(got) != 5 {
		t.Errorf("all warnings and errors: %d entries, want 5", len(got))
	}
	// The ring keeps the last mcpLogMax entries and the position survives it.
	for range mcpLogMax + 10 {
		l.record(mcpReply{Logs: []string{"dev warning: x"}})
	}
	if got, earlier := l.since(false, ""); len(got) != 100 || earlier != mcpLogMax-100 {
		t.Errorf("after the ring turned over: %d entries listed, %d earlier", len(got), earlier)
	}
}

func TestPerfSummary(t *testing.T) {
	ms := time.Millisecond
	fs := []mcpFrame{
		{Number: 3, Duration: 5 * ms, Phases: map[string]time.Duration{"input": 40 * ms, "build": 2 * ms, "layout": 3 * ms}},
		{Number: 4, Duration: 20 * ms, Phases: map[string]time.Duration{"build": 15 * ms, "layout": 4 * ms}},
	}
	got := perfSummary(fs, "the last 2 requested")
	for _, w := range []string{"2 frame(s), numbered 3 to 4", "25.00 ms of UI-thread time", "slowest frame 20.00 ms", "1 over the 16.7 ms", "Slowest phase: build 17.00 ms"} {
		if !strings.Contains(got, w) {
			t.Errorf("summary lacks %q: %s", w, got)
		}
	}
	if top := topPhases(fs[1].Phases, 1); len(top) != 1 || top[0] != "build 15.00 ms" {
		t.Errorf("topPhases %v", top)
	}
}

func TestDescribePrefer(t *testing.T) {
	m := mcpMount{Dark: true, Prefer: mcpPrefer{ReducedMotion: true, TextScale: 1.5, Locale: "ar", LocaleSet: true, Direction: "rtl"}}
	got := describePrefer(m)
	for _, w := range []string{"dark on", "reducedMotion on", "highContrast off", "textScale 1.5", "locale ar", "direction rtl"} {
		if !strings.Contains(got, w) {
			t.Errorf("%q lacks %q", got, w)
		}
	}
	if got := describePrefer(mcpMount{}); !strings.Contains(got, "textScale 1") || !strings.Contains(got, "the screen's own") {
		t.Errorf("defaults: %q", got)
	}
}

// The generated instance parses with and without the DevTools features, and
// only a tela that has them gets the code that needs them (an older tela
// would not compile it).
func TestGenerateMCPSourceFeatures(t *testing.T) {
	for _, f := range []genFeatures{{}, {DevTools: true}, {DevTools: true, I18N: true}} {
		src := string(generateMCPSource("widgets", contractPreviews, true, f))
		if _, err := parser.ParseFile(token.NewFileSet(), mcpGeneratedFile, src, parser.AllErrors); err != nil {
			t.Fatalf("%+v: does not parse: %v\n%s", f, err, src)
		}
		for what, present := range map[string]bool{
			"telatest.Instrument()":                    f.DevTools,
			"AriaSnapshotSources":                      f.DevTools,
			"telatest.Component":                       f.DevTools,
			`telah "github.com/kuronosu/tela/h"`:       f.DevTools,
			`telai18n "github.com/kuronosu/tela/i18n"`: f.I18N,
		} {
			if strings.Contains(src, what) != present {
				t.Errorf("%+v: %q present = %v", f, what, !present)
			}
		}
	}
	// The generated code of inspect asks for sources only with DevTools.
	if with, without := string(generateSource("widgets", contractPreview, false, genFeatures{DevTools: true})), string(generateSource("widgets", contractPreview, false)); !strings.Contains(with, "AriaSnapshotSources") || strings.Contains(without, "Instrument") {
		t.Errorf("inspect's generated code: with sources %v, without %v", strings.Contains(with, "AriaSnapshotSources"), strings.Contains(without, "Instrument"))
	}
}

// detectFeatures finds the telatest of the module and what the package
// links.
func TestDetectFeatures(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command is not in PATH")
	}
	if testing.Short() {
		t.Skip("runs go list")
	}
	if f := detectFeatures(goBin, "testdata/previews"); !f.DevTools || f.I18N {
		t.Errorf("testdata/previews: %+v, want DevTools without i18n", f)
	}
	if f := detectFeatures(goBin, "testdata/mcpapp"); !f.DevTools || !f.I18N {
		t.Errorf("testdata/mcpapp: %+v, want DevTools and i18n (the Prefs screen reads the locale)", f)
	}
	if f := detectFeatures(goBin, t.TempDir()); f.DevTools || f.I18N {
		t.Errorf("a directory outside any module: %+v", f)
	}
}
