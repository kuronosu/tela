package main

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
)

// demoPassword is the only password the fake sign-in accepts, with any
// well-formed e-mail. The wrong-password alert says so.
const demoPassword = "weave-2026"

// signInDelay is how long the fake request takes, to sign in or to create an
// account: long enough to see the spinner, short enough not to be annoying.
const signInDelay = 1500 * time.Millisecond

// minPassword is the shortest password a new account accepts.
const minPassword = 8

// bioLimit is the length of the bio in UTF-16 code units, the unit of
// HTML's maxlength and of h.MaxLength ("😀" counts 2).
const bioLimit = 160

// Mode is what the card asks for: the page has no routes, so the link under
// the form switches the same card between the two.
type Mode int

const (
	// SignIn is the "Welcome back" form: the zero value.
	SignIn Mode = iota
	// CreateAccount is the longer form for a new account.
	CreateAccount
)

// Status is where the request of the current mode is.
type Status int

const (
	// Idle is the form waiting for the user: the zero value.
	Idle Status = iota
	// Loading is the request in flight: the button shows a spinner.
	Loading
	// Failed is a rejected sign-in: the alert above the form says why.
	Failed
	// SignedIn replaces the sign-in form with a welcome.
	SignedIn
	// Created replaces the Create account form with a confirmation.
	Created
)

// Values is what the form holds. Page.Initial uses it to start a preview in
// any state; a real window starts with the zero Values. Sign in reads the
// e-mail, the password and Remember; Create account reads the e-mail, the
// password, Name, Bio and Terms.
type Values struct {
	Email    string
	Password string
	Remember bool
	Name     string
	Bio      string
	Terms    bool
}

// emailPattern is the "valid e-mail address" of the HTML standard, the same
// syntax h.Type("email") checks for the invalid: variant.
var emailPattern = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@" +
	"[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?" +
	"(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// EmailError is the message under the e-mail field, "" when it is fine.
func (v Values) EmailError() string {
	switch {
	case strings.TrimSpace(v.Email) == "":
		return "Enter your e-mail address."
	case !emailPattern.MatchString(v.Email):
		return "Enter a valid e-mail address, like you@example.com."
	}
	return ""
}

// PasswordError is the message under the password field, "" when it is fine.
func (v Values) PasswordError() string {
	if v.Password == "" {
		return "Enter your password."
	}
	return ""
}

// Valid reports whether the sign-in form can be sent.
func (v Values) Valid() bool { return v.EmailError() == "" && v.PasswordError() == "" }

// Accepts is the fake server: it knows one password.
func (v Values) Accepts() bool { return v.Valid() && v.Password == demoPassword }

// NameError is the message under the display name, "" when it is fine.
func (v Values) NameError() string {
	if strings.TrimSpace(v.Name) == "" {
		return "Enter your name."
	}
	return ""
}

// NewPasswordError is the message for the password of a new account, "" when
// it is fine. Sign in only needs a password to be typed.
func (v Values) NewPasswordError() string {
	if len(v.Password) < minPassword {
		return "Use at least 8 characters."
	}
	return ""
}

// CanCreate reports whether the Create account form can be sent: it keeps
// its button disabled until it can, so it has no errors to show on submit.
func (v Values) CanCreate() bool {
	return v.NameError() == "" && v.EmailError() == "" && v.NewPasswordError() == "" && v.Terms
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
