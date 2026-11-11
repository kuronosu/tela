// The .hyb reader and hyphenate below are ported from Android's Minikin
// (frameworks/minikin, Hyphenator.cpp; Chromium's copy is
// third_party/blink/renderer/platform/text/hyphenation/hyphenator_aosp.cc):
//
// Copyright (C) 2015 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package text

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// This file checks the hyphenation dictionaries against Chromium's: the
// .hyb files of Chromium's hyphen-data component (built by Minikin's
// mk_hyb_file.py from the same TeX patterns, AOSP's
// external/hyphenation-patterns), hyphenated by a port of Minikin's
// hyphenator. It runs where Chromium has downloaded them:
//
//	TELA_CHROMIUM_HYPHEN=~/.config/chromium/hyphen-data/120.0.6050.0 go test -run TestHyphenMinikin ./internal/text/
//
// over the words of /usr/share/dict/cracklib-small (TELA_WORDS) and random
// words over each alphabet. With TELA_UPDATE=1 it also writes the words of
// testdata/hyphenation/*.txt with Chromium's hyphenation, which
// TestHyphenGolden checks without Chromium.

// hybDict is a Minikin .hyb dictionary.
type hybDict struct {
	alpha              map[rune]uint32
	trie               []uint32
	charMask, linkMask uint32
	linkShift, patSh   uint32
	pat                []byte
}

func loadHyb(path string) (*hybDict, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	u := func(o uint32) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
	alphaOff, trieOff, patOff := u(8), u(12), u(16)
	d := &hybDict{alpha: map[rune]uint32{}}
	switch u(alphaOff) {
	case 0:
		lo, hi := u(alphaOff+4), u(alphaOff+8)
		for c := lo; c < hi; c++ {
			if v := b[alphaOff+12+c-lo]; v != 0 {
				d.alpha[rune(c)] = uint32(v)
			}
		}
	case 1:
		n := u(alphaOff + 4)
		for i := range n {
			e := u(alphaOff + 8 + 4*i)
			d.alpha[rune(e>>11)] = e & 0x7ff
		}
	default:
		return nil, fmt.Errorf("%s: unknown alphabet version", path)
	}
	d.charMask, d.linkShift, d.linkMask, d.patSh = u(trieOff+4), u(trieOff+8), u(trieOff+12), u(trieOff+16)
	n := u(trieOff + 20)
	d.trie = make([]uint32, n)
	for i := range d.trie {
		d.trie[i] = u(trieOff + 24 + 4*uint32(i))
	}
	d.pat = b[patOff:]
	return d, nil
}

// hyphenate is Minikin's Hyphenator::hyphenate for a word of BMP letters:
// result[i] is 1 when the word may break before word[i].
func (d *hybDict) hyphenate(word []rune) []uint8 {
	const minPrefix, minSuffix, maxSize = 2, 3, 64
	n := len(word)
	res := make([]uint8, n)
	padded := n + 2
	if n < minPrefix+minSuffix || padded > maxSize {
		return res
	}
	codes := make([]uint32, padded)
	for i, r := range word {
		c, ok := d.alpha[r]
		if !ok {
			return res
		}
		codes[i+1] = c
	}
	pu := func(o uint32) uint32 { return binary.LittleEndian.Uint32(d.pat[o:]) }
	patOffset := pu(8)
	maxOffset := padded - minSuffix - 1
	for i := 0; i < padded-1; i++ {
		node := uint32(0)
		for j := i; j < padded; j++ {
			c := codes[j]
			e := d.trie[node+c]
			if e&d.charMask != c {
				break
			}
			node = (e & d.linkMask) >> d.linkShift
			pi := d.trie[node] >> d.patSh
			if pi == 0 {
				continue
			}
			pe := pu(16 + pi*4)
			plen, pshift := int(pe>>26), int((pe>>20)&0x3f)
			buf := d.pat[patOffset+(pe&0xfffff):]
			off := j + 1 - (plen + pshift)
			for k := max(minPrefix-off, 0); k < min(plen, maxOffset-off); k++ {
				res[off+k] = max(res[off+k], buf[k])
			}
		}
	}
	for i := minPrefix; i < maxOffset; i++ {
		res[i] &= 1
	}
	return res
}

// hyphenCases are the dictionaries compared with Chromium's: the name of
// the .hyb file, the dictionary, words of the language (always in the
// golden) and the number of other words of the golden.
var hyphenCases = []struct {
	name   string
	dict   func() *Hyphenator
	words  string
	golden int
}{
	{"en-us", testHyphenUS, "hyphenation typography paragraph extraordinary internationalization considerations demonstrates characters representation table present project associate academy", 300},
	{"en-gb", testHyphenGB, "hyphenation typography paragraph extraordinary internationalisation colour programme behaviour", 200},
	{"es", testHyphenES, "hiperinflación latinoamericana desestabilizó extraordinariamente economías tipografía separación palabras computadora desarrollador internacionalización ferrocarril construcción murciélago pingüino lluvia carretera inconstitucionalidad otorrinolaringólogo", 250},
	{"fr", testHyphenFR, "typographie extraordinaire internationalisation développement anticonstitutionnellement aujourd’hui l’ordinateur bibliothèque œuvres connaissance", 200},
}

// hyphenWords returns the words compared for dictionary d: the words of
// its case, the words of the word list made of its letters, then random
// words of 5 to 24 of its letters.
func hyphenWords(t *testing.T, d *hyphDict, name, own string) []string {
	var letters []rune
	for r, l := range d.alpha {
		if r == l {
			letters = append(letters, r)
		}
	}
	slices.Sort(letters)
	words := strings.Fields(own)
	list := os.Getenv("TELA_WORDS")
	if list == "" {
		list = "/usr/share/dict/cracklib-small"
	}
	if f, err := os.Open(list); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			w := strings.TrimSpace(sc.Text())
			ok := len(w) > 0
			for _, r := range w {
				if l, in := d.alpha[r]; !in || l != r {
					ok = false
					break
				}
			}
			if ok {
				words = append(words, w)
			}
		}
		f.Close()
	} else {
		t.Logf("no word list (%v): random words only", err)
	}
	rng := rand.New(rand.NewPCG(uint64(len(name)), 270))
	for range 20000 {
		n := 5 + rng.IntN(20)
		w := make([]rune, n)
		for i := range w {
			w[i] = letters[rng.IntN(len(letters))]
		}
		words = append(words, string(w))
	}
	return words
}

func TestHyphenMinikin(t *testing.T) {
	dir := os.Getenv("TELA_CHROMIUM_HYPHEN")
	if dir == "" {
		t.Skip("set TELA_CHROMIUM_HYPHEN to Chromium's hyphen-data/<version> directory")
	}
	for _, c := range hyphenCases {
		t.Run(c.name, func(t *testing.T) {
			hyb, err := loadHyb(filepath.Join(dir, "hyph-"+c.name+".hyb"))
			if err != nil {
				t.Fatal(err)
			}
			h := c.dict()
			if err := h.Load(); err != nil {
				t.Fatal(err)
			}
			// The alphabets: the same letters in either case.
			for r, code := range hyb.alpha {
				if _, ok := h.dict.alpha[r]; !ok {
					t.Errorf("Chromium's alphabet has %q (code %d), ours does not", r, code)
				}
			}
			for r := range h.dict.alpha {
				if _, ok := hyb.alpha[r]; !ok {
					t.Errorf("our alphabet has %q, Chromium's does not", r)
				}
			}
			words := hyphenWords(t, h.dict, c.name, c.words)
			own := len(strings.Fields(c.words))
			bad := 0
			var golden []string
			for i, w := range words {
				rs := []rune(w)
				want := hyb.hyphenate(rs)
				var got []int32
				if len(rs) >= hyphenMinWord && len(rs) <= hyphenMaxWord {
					got = h.dict.points(rs, 0, nil)
				}
				gotRes := make([]uint8, len(rs))
				for _, p := range got {
					gotRes[p] = 1
				}
				if !slices.Equal(gotRes, want) {
					if bad++; bad <= 20 {
						t.Errorf("%q: %s, Chromium %s", w, showHyphens(rs, gotRes), showHyphens(rs, want))
					}
				}
				if i < own || (i%max(1, len(words)/c.golden) == 0 && len(golden) < own+c.golden) {
					golden = append(golden, w+"\t"+showHyphens(rs, want))
				}
			}
			t.Logf("%d words, %d differ", len(words), bad)
			if os.Getenv("TELA_UPDATE") == "1" {
				p := filepath.Join("testdata", "hyphenation", c.name+".txt")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				head := "# Chromium's hyphenation (hyphen-data hyph-" + c.name + ".hyb) of words; written by TestHyphenMinikin\n"
				if err := os.WriteFile(p, []byte(head+strings.Join(golden, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// showHyphens writes word with a hyphen before every letter res marks.
func showHyphens(word []rune, res []uint8) string {
	var b strings.Builder
	for i, r := range word {
		if res[i] != 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestHyphenGolden checks the dictionaries on words that Chromium
// hyphenated (TestHyphenMinikin writes them).
func TestHyphenGolden(t *testing.T) {
	for _, c := range hyphenCases {
		t.Run(c.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "hyphenation", c.name+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			h := c.dict()
			if err := h.Load(); err != nil {
				t.Fatal(err)
			}
			n := 0
			for line := range strings.Lines(string(data)) {
				line = strings.TrimSpace(line)
				if line == "" || line[0] == '#' {
					continue
				}
				word, want, ok := strings.Cut(line, "\t")
				if !ok {
					t.Fatalf("malformed line %q", line)
				}
				rs := []rune(word)
				got := make([]uint8, len(rs))
				if len(rs) >= hyphenMinWord && len(rs) <= hyphenMaxWord {
					for _, p := range h.dict.points(rs, 0, nil) {
						got[p] = 1
					}
				}
				if s := showHyphens(rs, got); s != want {
					t.Errorf("%s: %s, Chromium %s", word, s, want)
				}
				n++
			}
			if n < c.golden/2 {
				t.Errorf("only %d words", n)
			}
		})
	}
}
