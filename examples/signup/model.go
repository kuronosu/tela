package main

import (
	"regexp"
	"unicode/utf16"
)

// bioLimit is the length of the bio in UTF-16 code units, the unit of
// HTML's maxlength and of h.MaxLength ("😀" counts 2).
const bioLimit = 160

// minPassword is the shortest password the form accepts.
const minPassword = 8

// Values is what the form holds. Props.Initial uses it to start a preview in
// any state; a real window starts with the zero Values.
type Values struct {
	Name     string
	Email    string
	Password string
	Bio      string
	Terms    bool
}

// emailPattern is the "valid e-mail address" of the HTML standard, the same
// syntax h.Type("email") checks for the invalid: variant. Keeping the
// button's rule and the field's red border on one definition means they
// never disagree.
var emailPattern = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@" +
	"[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?" +
	"(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// ValidEmail reports whether s is an e-mail address by HTML's syntax.
func ValidEmail(s string) bool { return emailPattern.MatchString(s) }

// Valid reports whether the form can be submitted.
func (v Values) Valid() bool {
	return v.Name != "" && ValidEmail(v.Email) && len(v.Password) >= minPassword && v.Terms
}

// Strength rates a password from 0 (empty) to 4: one point each for the
// minimum length, a digit, upper and lower case, and a symbol or 12
// characters (at least 1 for any non-empty password).
func Strength(p string) int {
	if p == "" {
		return 0
	}
	var lower, upper, digit, other bool
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			other = true
		}
	}
	score := 0
	if len(p) >= minPassword {
		score++
	}
	if digit {
		score++
	}
	if lower && upper {
		score++
	}
	if other || len(p) >= 12 {
		score++
	}
	return max(score, 1)
}

// units is the length of s in UTF-16 code units.
func units(s string) int { return len(utf16.Encode([]rune(s))) }
