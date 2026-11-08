package memprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
)

const (
	// EnvVar is the environment variable that turns the measurement on and
	// picks its mode: "rss", or "profile".
	EnvVar = "TELA_MEM"
	// ProfilesEnvVar is a directory: when set, [Run] also writes the pprof
	// heap and allocs profiles of the case there.
	ProfilesEnvVar = "TELA_MEM_PROFILES"
	// ResultPrefix starts the line of a case's JSON result on stdout.
	ResultPrefix = "TELA_MEM_RESULT "
)

func init() {
	if Mode() == ModeProfile {
		runtime.MemProfileRate = 1
	}
}

// The modes of EnvVar.
const (
	ModeRSS     = "rss"
	ModeProfile = "profile"
)

// Mode returns the mode selected by EnvVar: "", ModeRSS or ModeProfile.
func Mode() string {
	switch v := os.Getenv(EnvVar); v {
	case "", "0":
		return ""
	case ModeProfile:
		return ModeProfile
	default:
		return ModeRSS
	}
}

// Stats is a reading of the process's memory, in bytes.
type Stats struct {
	// From runtime.MemStats.
	HeapInuse  uint64 `json:"heapInuse"`  // bytes in in-use spans
	HeapAlloc  uint64 `json:"heapAlloc"`  // bytes of allocated heap objects (live after a GC)
	HeapSys    uint64 `json:"heapSys"`    // heap bytes obtained from the OS
	Sys        uint64 `json:"sys"`        // total bytes obtained from the OS by the runtime
	TotalAlloc uint64 `json:"totalAlloc"` // cumulative bytes allocated
	NumGC      uint32 `json:"numGC"`

	// From /proc/self/status (Linux only; zero elsewhere).
	VmRSS    uint64 `json:"vmRSS"`
	RssAnon  uint64 `json:"rssAnon"`
	RssFile  uint64 `json:"rssFile"`
	RssShmem uint64 `json:"rssShmem"`
	VmHWM    uint64 `json:"vmHWM"` // peak RSS
}

// Read returns the current reading without collecting first.
func Read() Stats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := Stats{HeapInuse: m.HeapInuse, HeapAlloc: m.HeapAlloc, HeapSys: m.HeapSys, Sys: m.Sys, TotalAlloc: m.TotalAlloc, NumGC: m.NumGC}
	readProc(&s)
	return s
}

// Snapshot collects garbage, returns the freed memory to the OS and then
// reads the process's memory: the numbers are the live data plus the
// runtime's own structures, without the garbage a GC cycle would free.
func Snapshot() Stats {
	runtime.GC()
	debug.FreeOSMemory()
	return Read()
}

// readProc fills the RSS fields of s from /proc/self/status. The file does
// not exist on other systems, and the fields stay zero.
func readProc(s *Stats) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return
	}
	parseStatus(string(b), s)
}

// parseStatus reads the "Name:   123 kB" lines of /proc/self/status.
func parseStatus(status string, s *Stats) {
	for line := range strings.SplitSeq(status, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var dst *uint64
		switch name {
		case "VmRSS":
			dst = &s.VmRSS
		case "RssAnon":
			dst = &s.RssAnon
		case "RssFile":
			dst = &s.RssFile
		case "RssShmem":
			dst = &s.RssShmem
		case "VmHWM":
			dst = &s.VmHWM
		default:
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		n, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			n *= 1024
		}
		*dst = n
	}
}

// Mark is the live heap at a point of the case (after a GC).
type Mark struct {
	Label     string `json:"label"`
	HeapAlloc uint64 `json:"heapAlloc"`
}

// Probe tracks one case: the heap's peak and the bytes allocated since it
// started. [Run] makes it and hands it to the case.
type Probe struct {
	start     Stats
	peakAlloc uint64
	peakLive  uint64
	marks     []Mark
}

func newProbe() *Probe {
	p := &Probe{start: Snapshot()}
	p.peakAlloc = p.start.HeapAlloc
	p.peakLive = p.start.HeapAlloc
	return p
}

// Sample reads HeapAlloc and keeps the largest value seen. It includes
// garbage that a GC has not collected yet, so it is the heap's size as a
// window would have it. ReadMemStats stops the world: call it once per
// frame, not in a loop.
func (p *Probe) Sample() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p.peakAlloc = max(p.peakAlloc, m.HeapAlloc)
}

// Mark collects garbage and records the live heap under label, to see
// whether it grows through a session. It is expensive (a GC): call it a
// few times per case.
func (p *Probe) Mark(label string) {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p.marks = append(p.marks, Mark{Label: label, HeapAlloc: m.HeapAlloc})
	p.peakLive = max(p.peakLive, m.HeapAlloc)
	p.peakAlloc = max(p.peakAlloc, m.HeapAlloc)
}

// Case is one measured case of an example.
type Case struct {
	Example string  // "hello", "kit", ...
	Name    string  // "x1/rest"
	Width   float32 // logical size of the window
	Height  float32
	Scale   float64
	// Run builds the case (mounts the screen, plays what it plays) and calls
	// p.Sample and p.Mark where it wants the peak and the live heap read. It
	// returns whatever must stay alive until the case is measured (the
	// mounted app); the snapshot is taken with it still reachable.
	Run func(p *Probe) (keep any)
}

// Result is the JSON line of a case.
type Result struct {
	Example string  `json:"example"`
	Case    string  `json:"case"`
	Mode    string  `json:"mode"`
	Width   float32 `json:"width"`
	Height  float32 `json:"height"`
	Scale   float64 `json:"scale"`
	PhysW   int     `json:"physW"`
	PhysH   int     `json:"physH"`

	GoVersion  string `json:"goVersion"`
	GOMAXPROCS int    `json:"gomaxprocs"`
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`

	Before    Stats  `json:"before"` // the process before the case builds anything
	After     Stats  `json:"after"`  // after the case, collected, with the app alive
	PeakHeap  uint64 `json:"peakHeap"`
	PeakLive  uint64 `json:"peakLive"`
	Allocated uint64 `json:"allocated"` // bytes allocated during the case (TotalAlloc delta)
	Marks     []Mark `json:"marks,omitempty"`

	// Profile mode only.
	Breakdown *BreakdownResult `json:"breakdown,omitempty"`
}

// Run measures c when EnvVar is set and skips the test otherwise.
func Run(t *testing.T, c Case) {
	t.Helper()
	mode := Mode()
	if mode == "" {
		t.Skipf("memory measurement: set %s=rss or %s=profile; see go run ./internal/testutil/memreport", EnvVar, EnvVar)
	}
	if mode == ModeProfile && runtime.MemProfileRate != 1 {
		runtime.MemProfileRate = 1 // too late for what the process allocated already; the init normally did it
	}
	p := newProbe()
	keep := c.Run(p)
	p.Sample()
	after := Snapshot()
	r := Result{
		Example: c.Example, Case: c.Name, Mode: mode,
		Width: c.Width, Height: c.Height, Scale: c.Scale,
		PhysW: int(float64(c.Width)*c.Scale + 0.5), PhysH: int(float64(c.Height)*c.Scale + 0.5),
		GoVersion: runtime.Version(), GOMAXPROCS: runtime.GOMAXPROCS(0), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Before: p.start, After: after,
		PeakHeap: max(p.peakAlloc, after.HeapAlloc), PeakLive: max(p.peakLive, after.HeapAlloc),
		Allocated: after.TotalAlloc - p.start.TotalAlloc,
		Marks:     p.marks,
	}
	if mode == ModeProfile {
		b := Breakdown(DefaultRules)
		r.Breakdown = &b
	}
	if dir := os.Getenv(ProfilesEnvVar); dir != "" {
		if err := WriteProfiles(dir, c.Example+"_"+strings.ReplaceAll(c.Name, "/", "_")+"_"+mode); err != nil {
			t.Errorf("memprobe: %v", err)
		}
	}
	runtime.KeepAlive(keep)
	line, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("memprobe: %v", err)
	}
	fmt.Fprintf(os.Stdout, "\n%s%s\n", ResultPrefix, line)
}

// WriteProfiles writes the pprof heap and allocs profiles to
// dir/<name>.heap.pb.gz and dir/<name>.allocs.pb.gz, for
// `go tool pprof -sample_index=inuse_space|alloc_space`. The heap profile
// is as of the last GC.
func WriteProfiles(dir, name string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	runtime.GC()
	for _, kind := range []string{"heap", "allocs"} {
		f, err := os.Create(filepath.Join(dir, name+"."+kind+".pb.gz"))
		if err != nil {
			return err
		}
		err = pprof.Lookup(kind).WriteTo(f, 0)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("write the %s profile: %w", kind, err)
		}
	}
	return nil
}
