package core

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// mailboxRig drives a texMailbox by frame names and records what it lets
// go of.
type mailboxRig struct {
	m      texMailbox
	frames map[string]*texFrame
	names  map[*texFrame]string
}

func newMailboxRig() *mailboxRig {
	return &mailboxRig{frames: map[string]*texFrame{}, names: map[*texFrame]string{}}
}

func (r *mailboxRig) list(fs []*texFrame) string {
	var out []string
	for _, f := range fs {
		out = append(out, r.names[f])
	}
	slices.Sort(out)
	return strings.Join(out, " ")
}

func (r *mailboxRig) name(f *texFrame) string {
	if f == nil {
		return "-"
	}
	return r.names[f]
}

// state is "shown ready [waiting oldest first]".
func (r *mailboxRig) state() string {
	w := make([]string, len(r.m.waiting))
	for i, f := range r.m.waiting {
		w[i] = r.names[f]
	}
	return fmt.Sprintf("%s %s %v", r.name(r.m.shown), r.name(r.m.ready), w)
}

// do runs one operation, "a" (submit a finished), "a?" (submit a
// unfinished), "a!" (a's wait ends), "a~" (a's wait fails), "take" or
// "close", and returns the frames let go.
func (r *mailboxRig) do(op string) string {
	switch {
	case op == "take":
		_, old := r.m.take()
		if old == nil {
			return ""
		}
		return r.names[old]
	case op == "close":
		d, shown := r.m.close()
		if shown != nil {
			d = append(slices.Clone(d), shown)
		}
		return r.list(d)
	case strings.HasSuffix(op, "?"):
		return r.submit(strings.TrimSuffix(op, "?"), false)
	case strings.HasSuffix(op, "!"):
		d, ok := r.m.finish(r.frames[strings.TrimSuffix(op, "!")])
		if !ok {
			return "gone"
		}
		return r.list(d)
	case strings.HasSuffix(op, "~"):
		f := r.frames[strings.TrimSuffix(op, "~")]
		if !r.m.fail(f) {
			return "gone"
		}
		return r.names[f]
	}
	return r.submit(op, true)
}

func (r *mailboxRig) submit(name string, finished bool) string {
	f := &texFrame{}
	r.frames[name], r.names[f] = f, name
	return r.list(r.m.submit(f, finished))
}

func TestTextureMailboxRules(t *testing.T) {
	// Each step is an operation, the frames it lets go and the state after
	// it: shown, ready and the waiting ones.
	type step struct{ op, gone, state string }
	for _, tc := range []struct {
		name  string
		steps []step
	}{
		{"take the ready one", []step{
			{"take", "", "- - []"},
			{"a", "", "- a []"},
			{"take", "", "a - []"},
			{"b", "", "a b []"},
			{"take", "a", "b - []"},
			{"take", "", "b - []"}, // nothing new: the shown one stays
		}},
		{"a ready frame replaces the ready one", []step{
			{"a", "", "- a []"},
			{"b", "a", "- b []"},
			{"c", "b", "- c []"},
			{"take", "", "c - []"},
		}},
		{"a finished frame replaces the older waiting ones", []step{
			{"a?", "", "- - [a]"},
			{"b?", "", "- - [a b]"},
			{"c", "a b", "- c []"},
			{"a!", "gone", "- c []"},
			{"b~", "gone", "- c []"},
		}},
		{"the waits end in order", []step{
			{"a?", "", "- - [a]"},
			{"b?", "", "- - [a b]"},
			{"a!", "", "- a [b]"},
			{"take", "", "a - [b]"},
			{"b!", "", "a b []"},
			{"take", "a", "b - []"},
		}},
		{"a newer wait ends first", []step{
			{"a?", "", "- - [a]"},
			{"b?", "", "- - [a b]"},
			{"b!", "a", "- b []"},
			{"a!", "gone", "- b []"},
		}},
		{"a third waiting frame replaces the newer of the two", []step{
			{"a?", "", "- - [a]"},
			{"b?", "", "- - [a b]"},
			{"c?", "b", "- - [a c]"},
			{"d?", "c", "- - [a d]"},
			{"a!", "", "- a [d]"}, // the older one was left to finish
			{"e?", "", "- a [d e]"},
			{"f?", "e", "- a [d f]"},
			{"d!", "a", "- d [f]"},
		}},
		{"the ready one stays for the newer waiting ones", []step{
			{"a", "", "- a []"},
			{"b?", "", "- a [b]"},
			{"c?", "", "- a [b c]"},
			{"take", "", "a - [b c]"},
			{"b!", "", "a b [c]"},
			{"c!", "b", "a c []"},
		}},
		{"a failed wait drops its frame alone", []step{
			{"a?", "", "- - [a]"},
			{"b?", "", "- - [a b]"},
			{"a~", "a", "- - [b]"},
			{"a~", "gone", "- - [b]"},
			{"b!", "", "- b []"},
			{"b~", "gone", "- b []"},
		}},
		{"close lets go of everything", []step{
			{"a", "", "- a []"},
			{"take", "", "a - []"},
			{"b", "", "a b []"},
			{"c?", "", "a b [c]"},
			{"d?", "", "a b [c d]"},
			{"close", "a b c d", "- - []"},
			{"c!", "gone", "- - []"},
			{"close", "", "- - []"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newMailboxRig()
			for i, s := range tc.steps {
				gone := r.do(s.op)
				if state := r.state(); gone != s.gone || state != s.state {
					t.Fatalf("step %d, %s: let go of %q, state %q; want %q, %q", i, s.op, gone, state, s.gone, s.state)
				}
			}
		})
	}
}

// TestTextureMailboxInvariants plays random operations: the mailbox holds
// at most four frames, lets go of each one once, shows them in the order
// they were submitted, leaves the oldest waiting frame alone, and its
// counters add up.
func TestTextureMailboxInvariants(t *testing.T) {
	for seed := range uint64(200) {
		rng := rand.New(rand.NewPCG(seed, 7))
		r := newMailboxRig()
		gone := map[*texFrame]bool{}
		var lastShown uint64
		letGo := func(fs ...*texFrame) {
			for _, f := range fs {
				if f == nil {
					continue
				}
				if gone[f] {
					t.Fatalf("seed %d: frame %s let go twice", seed, r.names[f])
				}
				gone[f] = true
			}
		}
		var open []*texFrame
		for i := range 120 {
			var oldest *texFrame
			if len(r.m.waiting) > 0 {
				oldest = r.m.waiting[0]
			}
			unfinished := false
			switch k := rng.IntN(10); {
			case k < 3:
				f := &texFrame{}
				r.names[f] = fmt.Sprint(i)
				unfinished = true
				open = append(open, f)
				letGo(r.m.submit(f, false)...)
			case k < 4:
				f := &texFrame{}
				r.names[f] = fmt.Sprint(i)
				letGo(r.m.submit(f, true)...)
			case k < 7 && len(open) > 0:
				f := open[rng.IntN(len(open))]
				if d, ok := r.m.finish(f); ok {
					letGo(d...)
				} else if !gone[f] && f.state != texReady && f.state != texShown {
					t.Fatalf("seed %d: frame %s is neither waiting nor gone: %v", seed, r.names[f], f.state)
				}
			case k < 8 && len(open) > 0:
				if f := open[rng.IntN(len(open))]; r.m.fail(f) {
					letGo(f)
				}
			default:
				f, old := r.m.take()
				letGo(old)
				if f != nil {
					if f.seq <= lastShown {
						t.Fatalf("seed %d: frame %d shown after frame %d", seed, f.seq, lastShown)
					}
					lastShown = f.seq
				}
			}
			if unfinished && oldest != nil && gone[oldest] {
				t.Fatalf("seed %d: an unfinished frame replaced the oldest waiting one", seed)
			}
			held := r.m.pending()
			if r.m.shown != nil {
				held++
			}
			if held > 4 || len(r.m.waiting) > texMaxWaiting {
				t.Fatalf("seed %d: holds %d frames: %s", seed, held, r.state())
			}
			if r.m.submitted != r.m.shownN+r.m.droppedN+uint64(r.m.pending()) {
				t.Fatalf("seed %d: %d submitted, %d shown, %d dropped, %d pending", seed, r.m.submitted, r.m.shownN, r.m.droppedN, r.m.pending())
			}
		}
		d, shown := r.m.close()
		letGo(slices.Clone(d)...)
		letGo(shown)
		for f, name := range r.names {
			if !gone[f] {
				t.Fatalf("seed %d: frame %s never let go", seed, name)
			}
		}
	}
}
