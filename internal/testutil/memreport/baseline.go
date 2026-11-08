package main

import (
	"encoding/json"
	"os"
	"runtime"
	"time"

	"github.com/kuronosu/tela/internal/testutil/memprobe"
)

// baseline is the checked-in file the report compares with.
type baseline struct {
	Meta  meta                    `json:"meta"`
	Cases map[string]*caseNumbers `json:"cases"`
}

// meta records where the baseline was made: the numbers, RSS most of all,
// depend on the machine, the Go version and GOMAXPROCS.
type meta struct {
	Date       string `json:"date"`
	Machine    string `json:"machine"`
	GoVersion  string `json:"goVersion"`
	GOMAXPROCS int    `json:"gomaxprocs"`
}

// caseNumbers is what the baseline keeps of a case.
type caseNumbers struct {
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
	Scale  float64 `json:"scale"`
	PhysW  int     `json:"physW"`
	PhysH  int     `json:"physH"`

	HeapInuse uint64 `json:"heapInuse"`
	HeapAlloc uint64 `json:"heapAlloc"`
	HeapSys   uint64 `json:"heapSys"`
	Sys       uint64 `json:"sys"`
	PeakHeap  uint64 `json:"peakHeap"`
	PeakLive  uint64 `json:"peakLive"`
	Allocated uint64 `json:"allocated"`
	VmRSS     uint64 `json:"vmRSS"`
	RssAnon   uint64 `json:"rssAnon"`
	RssFile   uint64 `json:"rssFile"`
	RssShmem  uint64 `json:"rssShmem"`
	VmHWM     uint64 `json:"vmHWM"`

	// Breakdown is the live heap by category in bytes, from the profile run.
	Breakdown map[string]uint64 `json:"breakdown,omitempty"`
	// Marks is the live heap through the case (the scenes of a session).
	Marks []memprobe.Mark `json:"marks,omitempty"`
}

func numbersOf(r *caseResult) *caseNumbers {
	n := &caseNumbers{
		Width: r.rss.Width, Height: r.rss.Height, Scale: r.rss.Scale, PhysW: r.rss.PhysW, PhysH: r.rss.PhysH,
		HeapInuse: r.rss.After.HeapInuse, HeapAlloc: r.rss.After.HeapAlloc, HeapSys: r.rss.After.HeapSys, Sys: r.rss.After.Sys,
		PeakHeap: r.rss.PeakHeap, PeakLive: r.rss.PeakLive, Allocated: r.rss.Allocated,
		VmRSS: r.rss.After.VmRSS, RssAnon: r.rss.After.RssAnon, RssFile: r.rss.After.RssFile, RssShmem: r.rss.After.RssShmem, VmHWM: r.rss.After.VmHWM,
		Marks: r.rss.Marks,
	}
	if r.prof != nil && r.prof.Breakdown != nil {
		n.Breakdown = map[string]uint64{}
		for _, c := range r.prof.Breakdown.Categories {
			n.Breakdown[c.Name] = c.Bytes
		}
	}
	return n
}

func readBaseline(path string) (*baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var base baseline
	if err := json.Unmarshal(b, &base); err != nil {
		return nil, err
	}
	return &base, nil
}

// update replaces the cases of results in b and records where and when.
func (b *baseline) update(results []*caseResult, now time.Time) {
	if b.Cases == nil {
		b.Cases = map[string]*caseNumbers{}
	}
	for _, r := range results {
		if r.rss != nil {
			b.Cases[r.key] = numbersOf(r)
		}
	}
	b.Meta = meta{Date: now.Format("2006-01-02"), Machine: host(), GoVersion: runtime.Version(), GOMAXPROCS: runtime.GOMAXPROCS(0)}
}

func writeBaseline(path string, b *baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
