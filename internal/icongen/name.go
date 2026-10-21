package icongen

import (
	"fmt"
	"go/token"
	"strings"
	"unicode"
)

// VarName returns the exported Go identifier of the icon in the file rel,
// a slash-separated path relative to the icon directory ("arrow-up.svg",
// "solid/arrow_up.svg"). The extension is dropped, the rest is split at
// every character that is not a letter or a digit, and each piece starts
// with a capital: "arrow-up" is "ArrowUp", "volume_2" is "Volume2",
// "solid/arrow-up" is "SolidArrowUp". Digits that were apart stay apart
// with an underscore: "arrow-down-0-1" is "ArrowDown0_1" and "arrow-down-01"
// is "ArrowDown01". A name that does not start with a
// capital letter (a digit, a letter without case) gets "Icon" in front:
// "3d-rotate" is "Icon3dRotate". prefix and suffix are added around the
// result; prefix, when it is given, must itself start with a capital.
func VarName(rel, prefix, suffix string) (string, error) {
	base := rel
	if i := strings.LastIndexByte(base, '.'); i > strings.LastIndexByte(base, '/') && strings.EqualFold(base[i:], ".svg") {
		base = base[:i]
	}
	var sb strings.Builder
	var last rune
	for _, piece := range strings.FieldsFunc(base, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		rs := []rune(piece)
		if unicode.IsDigit(last) && unicode.IsDigit(rs[0]) {
			// Digits that were apart stay apart: arrow-down-0-1 and
			// arrow-down-01 are different icons.
			sb.WriteByte('_')
		}
		rs[0] = unicode.ToUpper(rs[0])
		sb.WriteString(string(rs))
		last = rs[len(rs)-1]
	}
	body := sb.String()
	if body == "" {
		return "", fmt.Errorf("the name %q has no letters or digits to make a Go identifier from", rel)
	}
	name := prefix + body + suffix
	if prefix == "" {
		if first := []rune(name)[0]; !unicode.IsUpper(first) {
			name = "Icon" + name
		}
	}
	if !token.IsExported(name) || !token.IsIdentifier(name) {
		return "", fmt.Errorf("the name %q makes %q, which is not an exported Go identifier", rel, name)
	}
	return name, nil
}

// checkAffix validates the -prefix and -suffix options.
func checkAffix(prefix, suffix string) error {
	if prefix != "" {
		if !token.IsIdentifier(prefix) || !token.IsExported(prefix) {
			return fmt.Errorf("invalid -prefix %q: it must start with a capital letter and have only letters, digits and underscores, for example -prefix Icon", prefix)
		}
	}
	if suffix != "" && !token.IsIdentifier("A"+suffix) {
		return fmt.Errorf("invalid -suffix %q: use letters, digits and underscores, for example -suffix Icon", suffix)
	}
	return nil
}
