package update

import (
	"errors"
	"fmt"
	"net/http"
)

// Config says where an app's updates are and whom to trust.
type Config struct {
	// URL is the manifest's URL (http or https; use https). Its signature
	// is at URL + ".sig". Artifact URLs in the manifest may be relative
	// to it.
	URL string
	// PublicKeys are the keys that may sign the manifest, as tela update
	// keygen prints them. Required.
	PublicKeys []PublicKey
	// Channel is the manifest's channel to follow; "" is "stable".
	Channel string
	// Policy, when set, overrides the activation of every release: an app
	// that always asks the user sets ActivateByApp.
	Policy Activation
	// Format is the install's package format ("appimage", "zip", "app",
	// "deb", "flatpak", "msix"); "" detects it from the running program
	// (see [Check]).
	Format string
	// HTTPClient makes the requests; nil is http.DefaultClient. Cancel
	// slow requests with the context given to Check and Download.
	HTTPClient *http.Client
}

func (c *Config) channel() string {
	if c.Channel == "" {
		return "stable"
	}
	return c.Channel
}

func (c *Config) client() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// Activation is when a downloaded update takes effect.
type Activation string

const (
	// ActivateNextStart: Apply puts the new version in place, and it runs
	// the next time the user starts the app. The default.
	ActivateNextStart Activation = "next-start"
	// ActivateByApp: the app decides when, for example with a "Restart to
	// update" banner that calls [Restart].
	ActivateByApp Activation = "app"
	// ActivateNow: the app restarts into the new version as soon as it is
	// applied, for urgent fixes. [UseUpdate] does that itself; code that
	// calls Check calls Restart after Apply.
	ActivateNow Activation = "now"
)

// known returns a, or ActivateNextStart for "" and unknown values.
func (a Activation) known() Activation {
	switch a {
	case ActivateByApp, ActivateNow:
		return a
	}
	return ActivateNextStart
}

var (
	// ErrUpToDate is returned by Check when no newer release is offered to
	// this install: none is newer, or the newer ones are not rolled out to
	// it, not built for it, or rolled back on it.
	ErrUpToDate = errors.New("update: up to date")
	// ErrEndOfSupport is matched (errors.Is) by an [*EndOfSupportError].
	ErrEndOfSupport = errors.New("update: this version is no longer supported")
	// ErrManaged is matched by a [*ManagedError]: a package manager or
	// store updates this install.
	ErrManaged = errors.New("update: the install is updated by its package manager")
	// ErrNotInstalled is returned when the running program is not an
	// install that updates itself or a managed one: go run, the output of
	// tela build, a binary copied by hand.
	ErrNotInstalled = errors.New("update: the running program is not an installed package")
	// ErrNoVersion is returned when the binary has no version: build it
	// with tela build or tela package at a tagged commit, or -version.
	ErrNoVersion = errors.New("update: the running program has no version")
	// ErrSignature: the manifest's signature is missing or invalid, or it
	// was made by a key the app does not trust.
	ErrSignature = errors.New("update: invalid manifest signature")
	// ErrExpired: the manifest's expiry date has passed (freeze
	// protection).
	ErrExpired = errors.New("update: the manifest has expired")
	// ErrOlderManifest: the manifest was published before the newest one
	// this install has seen (rollback protection).
	ErrOlderManifest = errors.New("update: the manifest is older than one already seen")
	// ErrHashMismatch: a download's size or SHA-256 differs from the
	// manifest's; the file is discarded.
	ErrHashMismatch = errors.New("update: the download does not match the manifest")
)

// EndOfSupportError is returned by Check when the running version is
// below the manifest's endOfSupport version.
type EndOfSupportError struct {
	// Version is the running version.
	Version string
	// Message and URL are the manifest's, to show the user.
	Message, URL string
}

func (e *EndOfSupportError) Error() string {
	s := fmt.Sprintf("update: version %s is no longer supported", e.Version)
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.URL != "" {
		s += " (" + e.URL + ")"
	}
	return s
}

// Is makes errors.Is(err, ErrEndOfSupport) true.
func (e *EndOfSupportError) Is(target error) bool { return target == ErrEndOfSupport }

// ManagedError is returned by Check for installs that something else
// updates.
type ManagedError struct {
	// Format is the install's: "deb", "flatpak" or "msix".
	Format string
	// Manager is what updates it: "apt" (deb), "flatpak", "msix" (the
	// package's .appinstaller file, or the Store).
	Manager string
}

func (e *ManagedError) Error() string {
	return fmt.Sprintf("update: this %s install is updated by %s", e.Format, e.Manager)
}

// Is makes errors.Is(err, ErrManaged) true.
func (e *ManagedError) Is(target error) bool { return target == ErrManaged }
