package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The DevTools tools of tela mcp (ADR-299 phase 28): state (why a
// component rendered and what its hooks hold), logs, perf (the frame
// profile) and prefer (the preferences of the user of the app). They need
// the DevTools additions of telatest (Instrument and the methods that read
// it): an instance built against an older tela says so and keeps the
// other tools.

// mcpNeedDevTools is the failure of a tool whose instance was built with a
// telatest that predates the DevTools additions.
const mcpNeedDevTools = "this package's tela module predates the DevTools additions of telatest (Instrument, Components, Frames); update it with go get github.com/kuronosu/tela@latest, and the tool works after the next rebuild"

// ---------------------------------------------------------------------
// logs

// mcpLogMax bounds the entries the server keeps.
const mcpLogMax = 500

// mcpLogEntry is a message of the screen or of the app.
type mcpLogEntry struct {
	Level  string // "error", "warn" or "info"
	Source string // "dev" (the dev mode of Tela), "app" (what it printed) or "telatest"
	Msg    string
}

// mcpLogs keeps the messages the instance sent with its replies, so that
// logs can list them after the call that reported them.
type mcpLogs struct {
	mu      sync.Mutex
	entries []mcpLogEntry
	dropped int // entries pushed out of the ring
	read    int // the position (dropped included) logs listed up to
}

// appLevel is how a line the app printed says its level: slog's text
// handler (level=WARN), its default handler (2025/01/01 10:00:00 ERROR x)
// and the log package's conventions.
var appLevel = regexp.MustCompile(`(?:^|\s)(?:level=)?(ERROR|WARN|WARNING|FATAL)\b|^panic:|^fatal error:`)

// record keeps the logs and the output of a reply.
func (l *mcpLogs) record(rep mcpReply) {
	var add []mcpLogEntry
	for _, m := range rep.Logs {
		m = strings.TrimSpace(m)
		switch {
		case m == "":
		case strings.HasPrefix(m, "dev message: "):
			add = append(add, mcpLogEntry{"error", "dev", strings.TrimPrefix(m, "dev message: ")})
		case strings.HasPrefix(m, "dev warning: "):
			add = append(add, mcpLogEntry{"warn", "dev", strings.TrimPrefix(m, "dev warning: ")})
		default:
			add = append(add, mcpLogEntry{"info", "telatest", strings.TrimPrefix(m, "telatest: ")})
		}
	}
	for _, line := range strings.Split(rep.Output, "\n") {
		line = strings.TrimRight(line, "\r ")
		if strings.TrimSpace(line) == "" {
			continue
		}
		level := "info"
		if m := appLevel.FindStringSubmatch(line); m != nil {
			level = "error"
			if strings.HasPrefix(m[1], "WARN") {
				level = "warn"
			}
		}
		add = append(add, mcpLogEntry{level, "app", line})
	}
	if len(add) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, add...)
	if over := len(l.entries) - mcpLogMax; over > 0 {
		l.entries = append([]mcpLogEntry(nil), l.entries[over:]...)
		l.dropped += over
	}
}

// since returns the entries not listed yet (all of them when all is
// true), at or above min ("", "warn" or "error"), and marks them listed.
func (l *mcpLogs) since(all bool, min string) (out []mcpLogEntry, earlier int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	from := 0
	if !all {
		from = max(l.read-l.dropped, 0)
	}
	rank := map[string]int{"info": 0, "warn": 1, "error": 2}
	for _, e := range l.entries[from:] {
		if rank[e.Level] >= rank[min] {
			out = append(out, e)
		}
	}
	l.read = l.dropped + len(l.entries)
	const show = 100
	if len(out) > show {
		earlier = len(out) - show
		out = out[earlier:]
	}
	return out, earlier
}

func (s *mcpSession) toolLogs(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Level string `json:"level"`
		All   bool   `json:"all"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	switch a.Level {
	case "", "info", "warn", "error":
	default:
		o.fail(fmt.Sprintf(`level %q: use "info" (everything), "warn" (warnings and errors) or "error"`, a.Level))
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	// What begin collected is in the log already, and listed below.
	o.errs, o.warns, o.other, o.output = nil, nil, nil, ""
	entries, earlier := s.logs.since(a.All, a.Level)
	if len(entries) == 0 {
		o.head = "No new log entries since the last logs call."
		if a.All {
			o.head = "The log is empty."
		}
		if a.Level != "" && a.Level != "info" {
			o.head = strings.TrimSuffix(o.head, ".") + " at level " + a.Level + " or above."
		}
		return o.result()
	}
	var b strings.Builder
	if earlier > 0 {
		fmt.Fprintf(&b, "(%d earlier entries left out)\n", earlier)
	}
	for _, e := range entries {
		fmt.Fprintf(&b, "[%s] %s: %s\n", e.Level, e.Source, e.Msg)
	}
	o.head = fmt.Sprintf("%d log entries since the last logs call: dev warnings and errors of Tela (dev), and what the app printed (app).", len(entries))
	o.sections = append(o.sections, "```\n"+strings.TrimRight(b.String(), "\n")+"\n```")
	return o.result()
}

// ---------------------------------------------------------------------
// state

func (s *mcpSession) toolState(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		elementArgs
		Max int `json:"max"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.Max < 0 || a.Max > 200 {
		o.fail("max must be between 1 and 200 components")
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "state", Max: a.Max}
	if a.given() {
		if a.Ref == "" {
			o.fail(`state names the element by its ref from the latest snapshot: {"ref": "e5"}; without one it lists the components of the screen`)
			return o.result()
		}
		e, err := s.element(ctx, &o, a.elementArgs)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		req.Loc = &e.loc
	}
	rep, ok := s.send(ctx, &o, req)
	if !ok {
		return o.result()
	}
	if !rep.DevTools {
		o.fail(mcpNeedDevTools)
		return o.result()
	}
	if len(rep.Components) == 0 {
		o.head = "No component of the screen has props or hooks."
		return o.result()
	}
	var b strings.Builder
	for i := range rep.Components {
		writeComponent(&b, &rep.Components[i], req.Loc != nil)
	}
	if req.Loc != nil {
		o.head = "The component that rendered the element (the nearest one above it); newest render first."
	} else {
		o.head = fmt.Sprintf("%d components with props or hooks, in tree order; give a ref for the one that rendered an element.", rep.Total)
		if rep.Total > len(rep.Components) {
			o.head += fmt.Sprintf(" The first %d are shown: max raises the limit.", len(rep.Components))
		}
	}
	o.sections = append(o.sections, strings.TrimRight(b.String(), "\n"))
	return o.result()
}

// writeComponent writes c as a section: its props, hooks and builds. A
// component of the whole screen is shortened (the last two renders).
func writeComponent(b *strings.Builder, c *mcpComponent, full bool) {
	fmt.Fprintf(b, "### %s", c.Name)
	if c.Key != "" {
		fmt.Fprintf(b, " (key %s)", c.Key)
	}
	if c.Source != "" {
		fmt.Fprintf(b, " made at %s", c.Source)
	}
	b.WriteString("\n")
	if len(c.Props) > 0 {
		var ps []string
		for _, p := range c.Props {
			ps = append(ps, p.Name+"="+p.Value)
		}
		fmt.Fprintf(b, "props: %s\n", strings.Join(ps, ", "))
	}
	for _, h := range c.Hooks {
		fmt.Fprintf(b, "hook #%d %s", h.Index+1, h.Kind)
		if h.Source != "" {
			fmt.Fprintf(b, " at %s", h.Source)
		}
		if h.Value != "" {
			fmt.Fprintf(b, " = %s", h.Value)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "built %d time(s)", c.Builds)
	renders := c.Renders
	if !full && len(renders) > 2 {
		renders = renders[len(renders)-2:]
	}
	if len(renders) > 0 {
		b.WriteString("; why, newest first:\n")
		for i := len(renders) - 1; i >= 0; i-- {
			fmt.Fprintf(b, "- frame %d: %s\n", renders[i].Frame, strings.Join(renders[i].Reasons, "; "))
		}
	} else {
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// ---------------------------------------------------------------------
// perf

func (s *mcpSession) toolPerf(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Frames int  `json:"frames"`
		Action bool `json:"action"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.Frames < 0 || a.Frames > 200 {
		o.fail("frames must be between 1 and 200")
		return o.result()
	}
	if a.Frames == 0 {
		a.Frames = 10
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	rep, ok := s.send(ctx, &o, mcpReq{Op: "perf", Count: a.Frames, FromAction: a.Action})
	if !ok {
		return o.result()
	}
	if !rep.DevTools {
		o.fail(mcpNeedDevTools)
		return o.result()
	}
	fs := rep.Frames
	what := fmt.Sprintf("the last %d requested", a.Frames)
	if a.Action {
		what = "the frames the last acting call caused"
	}
	if len(fs) == 0 {
		o.head = fmt.Sprintf("No frames in %s (the app is at frame %d: a screen that changes nothing draws none).", what, rep.FrameNow)
		return o.result()
	}
	o.head = perfSummary(fs, what)
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "- frame %d: %s", f.Number, millis(f.Duration))
		parts := topPhases(f.Phases, 4)
		if len(parts) > 0 {
			b.WriteString(" (" + strings.Join(parts, ", ") + ")")
		}
		fmt.Fprintf(&b, "; built %d, styled %d, laid out %d, repainted %d px\n", f.Built, f.Styled, f.LaidOut, f.Damage)
	}
	o.sections = append(o.sections, "```\n"+strings.TrimRight(b.String(), "\n")+"\n```\nTimes are those of this machine's headless render (no GPU, no display); compare frames with each other, and read the counts as the exact part.")
	return o.result()
}

// frameBudget is the time of a frame at 60 Hz.
const frameBudget = 16700 * time.Microsecond

func millis(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 2, 64) + " ms"
}

// topPhases lists the n phases that took the longest, "layout 1.20 ms".
func topPhases(phases map[string]time.Duration, n int) []string {
	type pd struct {
		name string
		d    time.Duration
	}
	var all []pd
	for k, d := range phases {
		all = append(all, pd{k, d})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].d > all[j].d || all[i].d == all[j].d && all[i].name < all[j].name })
	var out []string
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, all[i].name+" "+millis(all[i].d))
	}
	return out
}

// perfSummary is the line over the frames: the slowest phase, the slowest
// frame and how many go over the 60 Hz budget.
func perfSummary(fs []mcpFrame, what string) string {
	total := map[string]time.Duration{}
	var all, worst time.Duration
	over := 0
	for _, f := range fs {
		all += f.Duration
		worst = max(worst, f.Duration)
		if f.Duration > frameBudget {
			over++
		}
		for k, d := range f.Phases {
			total[k] += d
		}
	}
	delete(total, "input") // it runs between the frames, outside their duration
	slow := topPhases(total, 1)
	s := fmt.Sprintf("%d frame(s), numbered %d to %d (%s): %s of UI-thread time, slowest frame %s, %d over the 16.7 ms of 60 Hz.", len(fs), fs[0].Number, fs[len(fs)-1].Number, what, millis(all), millis(worst), over)
	if len(slow) > 0 {
		s += " Slowest phase: " + slow[0] + "."
	}
	return s
}

// ---------------------------------------------------------------------
// prefer

func (s *mcpSession) toolPrefer(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Dark          *bool    `json:"dark"`
		ReducedMotion *bool    `json:"reducedMotion"`
		HighContrast  *bool    `json:"highContrast"`
		TextScale     *float64 `json:"textScale"`
		Locale        *string  `json:"locale"`
		Direction     *string  `json:"direction"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.TextScale != nil && !(*a.TextScale > 0 && *a.TextScale <= 4) {
		o.fail("textScale must be above 0 and at most 4; 1 is the default")
		return o.result()
	}
	dir := ""
	if a.Direction != nil {
		switch d := strings.ToLower(strings.TrimSpace(*a.Direction)); d {
		case "ltr", "rtl":
			dir = d
		case "", "default":
		default:
			o.fail(fmt.Sprintf(`direction %q: use "ltr", "rtl" or "default" (the screen's own)`, *a.Direction))
			return o.result()
		}
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	m := s.mount
	if a.Dark == nil && a.ReducedMotion == nil && a.HighContrast == nil && a.TextScale == nil && a.Locale == nil && a.Direction == nil {
		o.head = "Current preferences: " + describePrefer(m) + `. Change them with {"reducedMotion": true}, {"textScale": 1.5}, {"locale": "ar", "direction": "rtl"}, {"dark": true}.`
		return o.result()
	}
	if a.Dark != nil {
		m.Dark = *a.Dark
	}
	if a.ReducedMotion != nil {
		m.Prefer.ReducedMotion = *a.ReducedMotion
	}
	if a.HighContrast != nil {
		m.Prefer.HighContrast = *a.HighContrast
	}
	if a.TextScale != nil {
		m.Prefer.TextScale = *a.TextScale
	}
	if a.Locale != nil {
		m.Prefer.Locale, m.Prefer.LocaleSet = strings.TrimSpace(*a.Locale), true
	}
	if a.Direction != nil {
		m.Prefer.Direction = dir
	}
	req := mcpReq{Op: "prefer", Dark: m.Dark, Prefer: &m.Prefer, Snapshot: true}
	before, prev := s.snapshotBefore(ctx, &o), s.mount
	rep, ok := s.send(ctx, &o, req)
	if rep.MountError != "" {
		// The screen could not be mounted with them: mount the previous
		// preferences again, so that the other tools keep working.
		if back, err := s.child.call(ctx, s.mountReq(false)); err == nil {
			o.absorb(back)
		}
		return o.result()
	}
	if !ok {
		return o.result()
	}
	if !rep.DevTools {
		o.fail(mcpNeedDevTools)
		return o.result()
	}
	s.mount = m
	if rep.Prefer != nil {
		s.mount.Prefer = *rep.Prefer
	}
	if rep.Remounted {
		s.remounted()
		s.rec.newSegment(s.mount, rep.Preview)
		// The snapshot of the reply is of the new mount: its refs are new.
		o.snapshot = nil
		if rep.YAML != nil {
			snap := s.withRefs(ctx, &o, *rep.YAML, rep.Nodes)
			o.snapshot = &snap
			s.noteSnapshot(*rep.YAML)
		}
	} else if lines := preferLines(prev, s.mount); len(lines) > 0 {
		s.rec.add(mcpStep{lines: lines, before: before, after: s.snapCur, w: s.curW, h: s.curH}, s.curPreview)
	}
	o.head = "Preferences now: " + describePrefer(s.mount) + ".\n" + "- " + strings.Join(rep.Applied, "\n- ")
	if len(rep.Applied) == 0 {
		o.head = "Preferences now: " + describePrefer(s.mount) + ". Nothing changed."
	}
	return o.result()
}

// describePrefer lists the preferences of a mount.
func describePrefer(m mcpMount) string {
	p := m.Prefer
	var parts []string
	flag := func(name string, on bool) {
		v := "off"
		if on {
			v = "on"
		}
		parts = append(parts, name+" "+v)
	}
	flag("dark", m.Dark)
	flag("reducedMotion", p.ReducedMotion)
	flag("highContrast", p.HighContrast)
	scale := p.TextScale
	if scale <= 0 {
		scale = 1
	}
	parts = append(parts, "textScale "+strconv.FormatFloat(scale, 'g', -1, 64))
	if p.LocaleSet {
		l := p.Locale
		if l == "" {
			l = "the system's"
		}
		parts = append(parts, "locale "+l)
	} else {
		parts = append(parts, "locale the app's own")
	}
	d := p.Direction
	if d == "" {
		d = "the screen's own"
	}
	parts = append(parts, "direction "+d)
	return strings.Join(parts, ", ")
}
