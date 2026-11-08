package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/testutil/memprobe"
)

func result(c string, heap uint64) *memprobe.Result {
	return &memprobe.Result{
		Example: "hello", Case: c, Mode: memprobe.ModeRSS, Width: 800, Height: 450, Scale: 2, PhysW: 1600, PhysH: 900,
		GoVersion: "go1.x", GOMAXPROCS: 4,
		After:    memprobe.Stats{HeapInuse: heap, HeapAlloc: heap, VmRSS: 2 * heap},
		PeakHeap: heap, Allocated: 3 * heap,
	}
}

func TestParseResult(t *testing.T) {
	line, _ := json.Marshal(result("x2/rest", 1<<20))
	out := "=== RUN   TestMemory\n\n" + memprobe.ResultPrefix + string(line) + "\n--- PASS: TestMemory (0.00s)\nPASS\n"
	r, err := parseResult(out)
	if err != nil || r.Case != "x2/rest" || r.After.HeapInuse != 1<<20 {
		t.Fatalf("parseResult = %+v, %v", r, err)
	}
	if _, err := parseResult("=== RUN   TestMemory\n--- SKIP: TestMemory\n"); err == nil || !strings.Contains(err.Error(), "skipped") {
		t.Errorf("a skipped case: error %v, want one that says it was skipped", err)
	}
	if _, err := parseResult(out + memprobe.ResultPrefix + string(line) + "\n"); err == nil {
		t.Error("two result lines: want an error")
	}
	if _, err := parseResult(memprobe.ResultPrefix + "{bad"); err == nil {
		t.Error("a bad line: want an error")
	}
}

func TestDelta(t *testing.T) {
	if d, over := delta(110<<20, 100<<20, 5); d != "+10.0 MiB (+10.0%)" || !over {
		t.Errorf("delta = %q, %v", d, over)
	}
	if d, over := delta(90<<20, 100<<20, 5); d != "-10.0 MiB (-10.0%)" || over {
		t.Errorf("delta = %q, %v", d, over)
	}
	if _, over := delta(110<<20, 100<<20, 0); over {
		t.Error("tolerance 0 flags nothing")
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	b := &baseline{}
	r := &caseResult{key: "hello/x2/rest", rss: result("x2/rest", 8<<20)}
	b.update([]*caseResult{r}, time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC))
	if err := writeBaseline(path, b); err != nil {
		t.Fatal(err)
	}
	got, err := readBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Date != "2026-11-07" || got.Cases["hello/x2/rest"].HeapInuse != 8<<20 {
		t.Errorf("baseline = %+v", got)
	}
	// A second update with another case keeps the first.
	got.update([]*caseResult{{key: "hello/x1/rest", rss: result("x1/rest", 4<<20)}}, time.Now())
	if len(got.Cases) != 2 {
		t.Errorf("update dropped a case: %d cases", len(got.Cases))
	}
}

func TestReport(t *testing.T) {
	r := &caseResult{key: "hello/x2/rest", rss: result("x2/rest", 8<<20)}
	r.prof = result("x2/rest", 8<<20)
	r.prof.Breakdown = &memprobe.BreakdownResult{Rate: 1, Total: 8 << 20, Categories: []memprobe.Category{{Name: memprobe.CatFonts, Bytes: 6 << 20}, {Name: memprobe.CatOther, Bytes: 2 << 20}}}
	base := &baseline{Cases: map[string]*caseNumbers{"hello/x2/rest": numbersOf(&caseResult{rss: result("x2/rest", 4<<20)})}}
	var out bytes.Buffer
	flagged := printReport(&out, []*caseResult{r}, base, 10, 0)
	if flagged == 0 {
		t.Error("a heap that doubled was not flagged")
	}
	for _, want := range []string{"hello/x2/rest", "heap in use", "fonts", "75.0%", "+4.0 MiB (+100.0%)", "Wayland", "1600x900"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestBackendDescribe(t *testing.T) {
	if got := backendTable[1].describe(1000, 1000); got != "7.6 MiB to 11.4 MiB (wl_shm, shared memory)" {
		t.Errorf("Wayland = %q", got)
	}
}
