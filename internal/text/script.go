package text

import "github.com/go-text/typesetting/language"

// resolveScripts sets out[i] to the script of text[i] for itemization
// (UAX #24 with the usual resolution of Common and Inherited, as in
// HarfBuzz's and Pango's itemizers): Inherited characters (combining
// marks) take the script of the preceding character; Common and Unknown
// characters take the script of the preceding text, except that a closing
// bracket takes the script of its opening bracket, and so does the text
// after it (paired brackets: "а (b) в" is Cyrillic "а (", Latin "b",
// Cyrillic ") в"); Common text before
// the first strong script takes that script. Text with no strong script
// at all is Common.
func resolveScripts(text []rune, out []language.Script) {
	type open struct {
		closer rune
		script language.Script
	}
	var stack []open
	cur := language.Script(0) // none yet
	firstUnresolved := -1
	for i, r := range text {
		s := language.LookupScript(r)
		switch s {
		case language.Inherited:
			s = cur
		case language.Common, language.Unknown:
			s = cur
			if c, ok := closingBracket(r); ok {
				if len(stack) == 64 {
					stack = stack[1:]
				}
				stack = append(stack, open{c, cur})
			} else if isClosingBracket(r) {
				for k := len(stack) - 1; k >= 0; k-- {
					if stack[k].closer == r {
						// The text after the pair continues in the
						// script of the pair.
						s = stack[k].script
						if s != 0 {
							cur = s
						}
						stack = stack[:k]
						break
					}
				}
			}
		default:
			if cur == 0 {
				// The first strong script also resolves the pending openers.
				for k := range stack {
					stack[k].script = s
				}
			}
			cur = s
		}
		if s == 0 && firstUnresolved < 0 {
			firstUnresolved = i
		}
		out[i] = s
	}
	fill := cur
	if fill == 0 {
		fill = language.Common
	}
	if firstUnresolved >= 0 {
		// Before the first strong script: take the first strong script of
		// the text (cur holds the last one; find the first).
		first := language.Script(0)
		for _, s := range out {
			if s != 0 {
				first = s
				break
			}
		}
		if first == 0 {
			first = fill
		}
		for i := range out {
			if out[i] == 0 {
				out[i] = first
			}
		}
	}
}

// bracketPairs are the opening and closing brackets used for script
// resolution (the common Bidi_Paired_Bracket pairs).
var bracketPairs = [...][2]rune{
	{'(', ')'}, {'[', ']'}, {'{', '}'}, {'«', '»'}, {'‹', '›'}, {'⁅', '⁆'},
	{'⁽', '⁾'}, {'₍', '₎'}, {'〈', '〉'}, {'⟨', '⟩'}, {'⟪', '⟫'}, {'〈', '〉'},
	{'《', '》'}, {'「', '」'}, {'『', '』'}, {'【', '】'}, {'〔', '〕'}, {'〖', '〗'},
	{'〘', '〙'}, {'〚', '〛'}, {'（', '）'}, {'［', '］'}, {'｛', '｝'}, {'｟', '｠'},
	{'｢', '｣'}, {'༺', '༻'}, {'༼', '༽'}, {'❨', '❩'}, {'❪', '❫'}, {'❬', '❭'},
	{'❮', '❯'}, {'❰', '❱'}, {'❲', '❳'}, {'❴', '❵'},
}

func closingBracket(r rune) (rune, bool) {
	for _, p := range bracketPairs {
		if p[0] == r {
			return p[1], true
		}
	}
	return 0, false
}

func isClosingBracket(r rune) bool {
	for _, p := range bracketPairs {
		if p[1] == r {
			return true
		}
	}
	return false
}
