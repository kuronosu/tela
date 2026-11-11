package text

import (
	"unicode"

	"github.com/go-text/typesetting/language"
)

// paraScope is what the system font fallback knows of the paragraph a
// cluster belongs to, so that a script is drawn by one installed family
// throughout the paragraph: Japanese text with a few Simplified Chinese
// characters takes one family that has them all rather than alternating
// between a Japanese font and a Chinese one, character by character, and
// its kana take the family of its Han (one group, scriptGroup). Chromium
// falls back per character among each OS's own CJK fonts, which cover all
// of common text; over whatever families are installed, a family per
// paragraph is the browser's look.
//
// A scope belongs to one layout and is used by the goroutine that shapes
// it (Layout.shape holds shapeMu), so it has no lock.
type paraScope struct {
	runes   []rune
	scripts []language.Script // per rune: its run's script (resolveScripts)
	// cands returns the font candidates of rune i's style, or nil for a
	// rune that is not text (an inline box).
	cands func(i int) *candidates
	picks map[pickKey]string
}

type pickKey struct {
	cs    *candidates
	group language.Script
}

// pick returns the normalized name of the installed family that draws the
// script group in this paragraph for the query of cs: among the families
// that support the group's scripts in the paragraph, the first by famRank
// for req that covers every character of the group that the registered
// fonts lack, else the one that covers most of them; "" when the
// registered fonts cover everything or no family covers anything. The
// characters are the paragraph's runes of the group's scripts and the CJK
// punctuation of its runs, in the text of the same query.
func (p *paraScope) pick(cs *candidates, group language.Script, req fbRequest) string {
	if group == 0 {
		return ""
	}
	k := pickKey{cs, group}
	if name, ok := p.picks[k]; ok {
		return name
	}
	seen := map[rune]bool{}
	var need []rune
	for i, r := range p.runes {
		if seen[r] || scriptGroup(p.scripts[i]) != group || !groupRune(r, group) || p.cands(i) != cs {
			continue
		}
		seen[r] = true
		if !cs.registeredHas(r) {
			need = append(need, r)
		}
	}
	name := ""
	if len(need) > 0 {
		name = cs.sys.pickFamily(need, group, req, cs.q)
	}
	if p.picks == nil {
		p.picks = map[pickKey]string{}
	}
	p.picks[k] = name
	return name
}

// groupRune reports whether r counts for the family of a script group:
// its own script is in the group, or, for the CJK group, it is CJK
// punctuation (CJK Symbols and Punctuation, the prolonged sound mark, the
// halfwidth and fullwidth forms), which Japanese and Chinese fonts draw in
// their own style. Spaces, digits and Latin letters in a CJK run do not:
// the registered fonts usually have them, and any CJK font does.
func groupRune(r rune, group language.Script) bool {
	if isIgnorable(r) || unicode.IsSpace(r) || unicode.IsControl(r) {
		return false
	}
	if scriptGroup(language.LookupScript(r)) == group {
		return true
	}
	return group == language.Han && (r >= 0x3000 && r <= 0x303F || r >= 0xFF00 && r <= 0xFFEF || r == 0x30FC || r == 0x30A0)
}

// registeredHas reports whether a registered family of cs (requested, of
// the rune's script, or any other) has r.
func (cs *candidates) registeredHas(r rune) bool {
	has := func(f *Face) bool { return f.HasRune(r) }
	return cs.first(cs.primary, has) != nil || cs.first(cs.scriptSlots(clusterScript([]rune{r}, language.Common)), has) != nil ||
		cs.first(cs.rest, has) != nil
}

// pickFamily returns the family of paraScope.pick for the characters need
// of a script group: the families that support a script of need (or the
// group's, when every character is punctuation), by famRank for req (with
// Japanese families first when need has kana, fbRequest.forScript); the
// first whose font for q (CSS matching among the family's fonts) covers
// all of need, else the first that covers most of it, else "".
func (s *systemFonts) pickFamily(need []rune, group language.Script, req fbRequest, q Query) string {
	s.ensureIndex()
	scripts := map[language.Script]bool{}
	for _, r := range need {
		if sc := language.LookupScript(r); usesScriptList(sc) {
			scripts[sc] = true
			if sc != language.Han {
				// Text with kana is Japanese, Hangul Korean.
				req = req.forScript(sc)
			}
		}
	}
	if len(scripts) == 0 {
		scripts[group] = true
	}
	set := map[string]bool{}
	var names []string
	for sc := range scripts {
		for _, n := range s.scriptSupport(sc) {
			if !set[n] {
				set[n] = true
				names = append(names, n)
			}
		}
	}
	best, bestN := "", 0
	for _, name := range s.sortRanked(names, req) {
		idxs := s.byFamily[name]
		if len(idxs) == 0 {
			continue
		}
		e := &s.entries[idxs[s.matchEntry(idxs, q)]]
		n := 0
		for _, r := range need {
			if e.runes.Contains(r) {
				n++
			}
		}
		if n == len(need) {
			return name
		}
		if n > bestN {
			best, bestN = name, n
		}
	}
	return best
}
