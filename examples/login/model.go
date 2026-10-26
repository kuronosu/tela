package main

import (
	"regexp"
	"strings"
	"time"
)

// demoPassword is the only password the fake sign-in accepts, with any
// well-formed e-mail. The wrong-password alert says so.
const demoPassword = "weave-2026"

// signInDelay is how long the fake request takes: long enough to see the
// spinner, short enough not to be annoying.
const signInDelay = 1500 * time.Millisecond

// Status is where the sign-in is.
type Status int

const (
	// Idle is the form waiting for the user: the zero value.
	Idle Status = iota
	// Loading is the request in flight: the button shows a spinner.
	Loading
	// Failed is a rejected request: the alert above the form says why.
	Failed
	// SignedIn replaces the form with a welcome.
	SignedIn
)

// Values is what the form holds. Page.Initial uses it to start a preview in
// any state; a real window starts with the zero Values.
type Values struct {
	Email    string
	Password string
	Remember bool
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

// Valid reports whether the form can be sent.
func (v Values) Valid() bool { return v.EmailError() == "" && v.PasswordError() == "" }

// Accepts is the fake server: it knows one password.
func (v Values) Accepts() bool { return v.Valid() && v.Password == demoPassword }
