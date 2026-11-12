package idmap

import (
	"math/rand/v2"
	"testing"
)

// check fails unless m holds exactly want.
func check(t *testing.T, m Map[int], want map[uint64]int) {
	t.Helper()
	if m.Len() != len(want) {
		t.Fatalf("Len %d, want %d", m.Len(), len(want))
	}
	for k, v := range want {
		if got, ok := m.Get(k); !ok || got != v {
			t.Fatalf("Get(%d) = %d, %v; want %d", k, got, ok, v)
		}
	}
	n := 0
	m.Range(func(k uint64, v int) bool {
		n++
		if w, ok := want[k]; !ok || w != v {
			t.Fatalf("Range: %d = %d, want %d (%v)", k, v, w, ok)
		}
		return true
	})
	if n != len(want) {
		t.Fatalf("Range visited %d keys, want %d", n, len(want))
	}
}

// TestMapAgainstGoMap applies random sets and deletes (dense IDs, as core
// numbers nodes, and sparse ones, as the remote host shifts them) to a Map
// and a Go map, keeping every version: old versions do not change.
func TestMapAgainstGoMap(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, keys := range []func() uint64{
		func() uint64 { return uint64(r.IntN(3000)) },
		func() uint64 { return uint64(r.IntN(50))<<40 | uint64(r.IntN(100)) },
		func() uint64 { return r.Uint64() },
	} {
		var m Map[int]
		want := map[uint64]int{}
		type version struct {
			m    Map[int]
			want map[uint64]int
		}
		var versions []version
		for step := range 20000 {
			k := keys()
			if r.IntN(3) == 0 {
				m = m.Delete(k)
				delete(want, k)
			} else {
				m = m.Set(k, step)
				want[k] = step
			}
			if step%2000 == 0 {
				c := make(map[uint64]int, len(want))
				for k, v := range want {
					c[k] = v
				}
				versions = append(versions, version{m, c})
			}
		}
		check(t, m, want)
		for _, v := range versions {
			check(t, v.m, v.want)
		}
	}
}

// TestBuilder: a Builder's updates do not change the maps it started from
// or returned before.
func TestBuilder(t *testing.T) {
	var base Map[int]
	for i := range 1000 {
		base = base.Set(uint64(i), i)
	}
	b := NewBuilder(base)
	for i := range 2000 {
		b.Set(uint64(i), -i)
	}
	mid := b.Map()
	for i := 0; i < 2000; i += 2 {
		b.Delete(uint64(i))
	}
	b.Set(5000, 5)
	end := b.Map()
	want := map[uint64]int{}
	for i := range 1000 {
		want[uint64(i)] = i
	}
	check(t, base, want)
	for i := range 2000 {
		want[uint64(i)] = -i
	}
	check(t, mid, want)
	for i := 0; i < 2000; i += 2 {
		delete(want, uint64(i))
	}
	want[5000] = 5
	check(t, end, want)
	for i := range 3000 {
		end = end.Delete(uint64(i))
	}
	end = end.Delete(5000)
	if end.Len() != 0 || end.root != nil {
		t.Errorf("all deleted: Len %d, root %v", end.Len(), end.root)
	}
}

func BenchmarkGet10k(b *testing.B) {
	var m Map[int]
	for i := range 10000 {
		m = m.Set(uint64(i+1), i)
	}
	b.ReportAllocs()
	k := uint64(0)
	for b.Loop() {
		k = k%10000 + 1
		m.Get(k)
	}
}

func BenchmarkSet10k(b *testing.B) {
	var m Map[int]
	for i := range 10000 {
		m = m.Set(uint64(i+1), i)
	}
	b.ReportAllocs()
	k := uint64(0)
	for b.Loop() {
		k = k%10000 + 1
		m.Set(k, 1)
	}
}
