package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/kuronosu/tela/internal/testutil/memprobe"
)

func mib(b uint64) string { return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20)) }

func mibNum(b uint64) string { return fmt.Sprintf("%.1f", float64(b)/(1<<20)) }

// delta formats now against the baseline value base: "+0.2 MiB (+1.6%)", and
// whether it grew over tolerance percent (0: never).
func delta(now, base uint64, tolerance float64) (text string, over bool) {
	d := int64(now) - int64(base)
	sign := "+"
	if d < 0 {
		sign, d = "-", -d
	}
	pct := ""
	if base > 0 {
		p := 100 * float64(int64(now)-int64(base)) / float64(base)
		pct = fmt.Sprintf(" (%+.1f%%)", p)
		over = tolerance > 0 && p > tolerance
	}
	return fmt.Sprintf("%s%s%s", sign, mib(uint64(d)), pct), over
}

// printReport writes the tables and returns how many numbers grew over the
// tolerance.
func printReport(w io.Writer, results []*caseResult, base *baseline, tolerance float64, sites int) (flagged int) {
	var ok []*caseResult
	for _, r := range results {
		if r.err == nil && r.rss != nil {
			ok = append(ok, r)
		}
	}
	if len(ok) == 0 {
		return 0
	}
	first := ok[0].rss
	fmt.Fprintf(w, "Memory report (ADR-240): %s, %s, GOMAXPROCS=%d\n", host(), first.GoVersion, first.GOMAXPROCS)
	if len(base.Cases) > 0 {
		fmt.Fprintf(w, "Baseline: %s on %s, %s, GOMAXPROCS=%d", base.Meta.Date, base.Meta.Machine, base.Meta.GoVersion, base.Meta.GOMAXPROCS)
		if base.Meta.GOMAXPROCS != first.GOMAXPROCS || base.Meta.GoVersion != first.GoVersion {
			fmt.Fprint(w, "  <- differs from this run: the raster buffers, the RSS and the Go runtime's numbers move with it")
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "Baseline: none (-update writes one)")
	}
	fmt.Fprintln(w, "All sizes in MiB. Headless stands in for one real backend's presentation buffer (see the end).")

	summary(w, ok)
	for _, r := range ok {
		flagged += detail(w, r, base.Cases[r.key], tolerance, sites)
	}
	backends(w, ok)
	if flagged > 0 {
		fmt.Fprintf(w, "\n%d numbers grew over the baseline by more than %.0f%% (marked !)\n", flagged, tolerance)
	}
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(w, "\nFAILED %s: %v\n", r.key, r.err)
		}
	}
	return flagged
}

// summaryCats are the columns of the summary's breakdown.
var summaryCats = []struct{ head, cat string }{
	{"fonts", memprobe.CatFonts},
	{"glyphs", memprobe.CatGlyphCache},
	{"fb+copy", memprobe.CatFramebuffer},
	{"scratch", memprobe.CatRasterBuf},
	{"backdrop", memprobe.CatBackdrop},
	{"images", memprobe.CatImages},
	{"rest", ""},
}

func summary(w io.Writer, ok []*caseResult) {
	fmt.Fprintf(w, "\n%-24s %-11s | %8s %8s %8s | %8s %8s %8s | live heap by subsystem (profile run)\n", "case", "size px", "in use", "peak", "alloc'd", "RSS", "anon", "file")
	fmt.Fprintf(w, "%-24s %-11s | %26s | %26s | ", "", "", "", "")
	for _, c := range summaryCats {
		fmt.Fprintf(w, "%8s ", c.head)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("-", 24+1+11+3+26+3+26+3+9*len(summaryCats)))
	for _, r := range ok {
		a := r.rss.After
		fmt.Fprintf(w, "%-24s %-11s | %8s %8s %8s | %8s %8s %8s | ", r.key, fmt.Sprintf("%dx%d", r.rss.PhysW, r.rss.PhysH),
			mibNum(a.HeapInuse), mibNum(r.rss.PeakHeap), mibNum(r.rss.Allocated), mibNum(a.VmRSS), mibNum(a.RssAnon), mibNum(a.RssFile))
		if r.prof != nil && r.prof.Breakdown != nil {
			by := map[string]uint64{}
			named := uint64(0)
			for _, c := range r.prof.Breakdown.Categories {
				by[c.Name] = c.Bytes
			}
			for _, c := range summaryCats {
				if c.cat == "" {
					fmt.Fprintf(w, "%8s ", mibNum(r.prof.Breakdown.Total-named))
					continue
				}
				named += by[c.cat]
				fmt.Fprintf(w, "%8s ", mibNum(by[c.cat]))
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "in use = HeapInuse after GC; peak = largest HeapAlloc during the case (garbage included); alloc'd = bytes allocated during the case.")
	fmt.Fprintln(w, "RSS = VmRSS after GC and FreeOSMemory (anon = heap and stacks, file = the binary's pages). rest = display lists, images, text layout, trees, other.")
}

// row prints one metric against its baseline.
func row(w io.Writer, name string, now uint64, base *caseNumbers, get func(*caseNumbers) uint64, tolerance float64) (flagged int) {
	if base == nil {
		fmt.Fprintf(w, "  %-34s %12s\n", name, mib(now))
		return 0
	}
	b := get(base)
	d, over := delta(now, b, tolerance)
	mark := " "
	if over {
		mark, flagged = "!", 1
	}
	fmt.Fprintf(w, "  %-34s %12s %12s  %s %s\n", name, mib(now), mib(b), mark, d)
	return flagged
}

func detail(w io.Writer, r *caseResult, base *caseNumbers, tolerance float64, sites int) (flagged int) {
	res := r.rss
	fmt.Fprintf(w, "\n== %s: %gx%g logical, scale %g, %dx%d px\n", r.key, res.Width, res.Height, res.Scale, res.PhysW, res.PhysH)
	if base == nil {
		fmt.Fprintf(w, "  %-34s %12s\n", "", "now")
	} else {
		fmt.Fprintf(w, "  %-34s %12s %12s    %s\n", "", "now", "baseline", "delta")
	}
	a := res.After
	// f's numbers are checked against the tolerance; g's move with the
	// timing of the garbage collector or the OS (the peak, what the runtime
	// asked the OS for, the binary's pages) and are only shown.
	f := func(name string, v uint64, get func(*caseNumbers) uint64) {
		flagged += row(w, name, v, base, get, tolerance)
	}
	g := func(name string, v uint64, get func(*caseNumbers) uint64) {
		row(w, name, v, base, get, 0)
	}
	f("heap in use (HeapInuse)", a.HeapInuse, func(n *caseNumbers) uint64 { return n.HeapInuse })
	f("heap alloc (live objects)", a.HeapAlloc, func(n *caseNumbers) uint64 { return n.HeapAlloc })
	g("heap peak during the case", res.PeakHeap, func(n *caseNumbers) uint64 { return n.PeakHeap })
	f("live heap peak (at the marks)", res.PeakLive, func(n *caseNumbers) uint64 { return n.PeakLive })
	f("allocated during the case", res.Allocated, func(n *caseNumbers) uint64 { return n.Allocated })
	g("heap from the OS (HeapSys)", a.HeapSys, func(n *caseNumbers) uint64 { return n.HeapSys })
	g("runtime total (Sys)", a.Sys, func(n *caseNumbers) uint64 { return n.Sys })
	if a.VmRSS > 0 {
		f("RSS", a.VmRSS, func(n *caseNumbers) uint64 { return n.VmRSS })
		f("  anon (heap, stacks)", a.RssAnon, func(n *caseNumbers) uint64 { return n.RssAnon })
		g("  file (binary, mapped files)", a.RssFile, func(n *caseNumbers) uint64 { return n.RssFile })
		g("  shmem", a.RssShmem, func(n *caseNumbers) uint64 { return n.RssShmem })
		g("peak RSS (VmHWM, whole process)", a.VmHWM, func(n *caseNumbers) uint64 { return n.VmHWM })
	}
	fmt.Fprintf(w, "  before the case: heap alloc %s, RSS %s (the test binary's own)\n", mib(res.Before.HeapAlloc), mib(res.Before.VmRSS))

	if r.prof != nil && r.prof.Breakdown != nil {
		b := r.prof.Breakdown
		fmt.Fprintf(w, "  live heap by subsystem (profile run, MemProfileRate=%d, %s recorded):\n", b.Rate, mib(b.Total))
		for _, c := range b.Categories {
			if c.Bytes == 0 && (base == nil || base.Breakdown[c.Name] == 0) {
				continue
			}
			pct := 0.0
			if b.Total > 0 {
				pct = 100 * float64(c.Bytes) / float64(b.Total)
			}
			line := fmt.Sprintf("    %-32s %10s %5.1f%%", c.Name, mib(c.Bytes), pct)
			if base != nil && base.Breakdown != nil {
				d, over := delta(c.Bytes, base.Breakdown[c.Name], tolerance)
				mark := " "
				if over {
					mark = "!"
					flagged++
				}
				line += fmt.Sprintf("   %s vs %s  %s %s", "", mib(base.Breakdown[c.Name]), mark, d)
			}
			fmt.Fprintln(w, line)
		}
	}
	if len(res.Marks) > 1 {
		fmt.Fprintf(w, "  live heap after each stretch (GC at the mark):\n")
		prev := uint64(0)
		for i, m := range res.Marks {
			d := ""
			if i > 0 {
				d = fmt.Sprintf("%+.1f", (float64(m.HeapAlloc)-float64(prev))/(1<<20))
			}
			fmt.Fprintf(w, "    %-22s %10s %8s\n", m.Label, mib(m.HeapAlloc), d)
			prev = m.HeapAlloc
		}
	}
	if sites > 0 && r.prof != nil && r.prof.Breakdown != nil {
		fmt.Fprintf(w, "  largest allocation sites of the live heap:\n")
		for i, s := range r.prof.Breakdown.Top {
			if i >= sites {
				break
			}
			fmt.Fprintf(w, "    %10s  %6d objs  [%s]\n", mib(s.Bytes), s.Objects, s.Category)
			shown := 0
			for _, fr := range s.Frames {
				if strings.HasPrefix(fr, "runtime.") || strings.HasPrefix(fr, "internal/runtime/") {
					continue
				}
				fmt.Fprintf(w, "        %s\n", fr)
				if shown++; shown == 6 {
					break
				}
			}
		}
	}
	return flagged
}

// backends lists, for each distinct physical size of the cases, the
// presentation buffers of the real backends.
func backends(w io.Writer, ok []*caseResult) {
	fmt.Fprintf(w, "\nPresentation buffers of the real backends, by physical size (computed from the backends' code, on top of the app's framebuffer; headless keeps one copy and stands in for one of them):\n")
	seen := map[[2]int]bool{}
	for _, r := range ok {
		size := [2]int{r.rss.PhysW, r.rss.PhysH}
		if seen[size] {
			continue
		}
		seen[size] = true
		fmt.Fprintf(w, "  %dx%d px (one frame = %s):\n", size[0], size[1], mib(frameBytes(size[0], size[1])))
		for _, b := range backendTable {
			fmt.Fprintf(w, "    %-34s copies: %-22s %s\n", b.name, b.count, b.describe(size[0], size[1]))
		}
	}
}
