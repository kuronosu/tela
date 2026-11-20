package oauth

import (
	"errors"
	"fmt"
)

var (
	// ErrSignedOut is returned when there is no signed-in account: no
	// tokens are stored, or the server refused to refresh them
	// (invalid_grant), after which they were deleted. Ask the user to
	// sign in again.
	ErrSignedOut = errors.New("oauth: signed out")

	// ErrAccessDenied is matched by errors.Is when the user, or the server,
	// refused the sign-in (error=access_denied).
	ErrAccessDenied = errors.New("oauth: access denied")

	// ErrExpired is matched by errors.Is when a device code expired before
	// the user approved it (error=expired_token).
	ErrExpired = errors.New("oauth: the code expired")

	// ErrStateMismatch is returned by SignIn when the redirect carried a
	// state other than the one the app sent: not the answer to this
	// request (RFC 9700 §4.7). The sign-in is abandoned.
	ErrStateMismatch = errors.New("oauth: the redirect's state does not match the request's")
)

// Error is an error response of the server (RFC 6749 §5.2, §4.1.2.1,
// RFC 8628 §3.5), found with errors.As.
type Error struct {
	// Code is the error code, such as "access_denied" or "invalid_grant".
	Code string
	// Description and URI are the server's optional explanation. Show
	// them to a developer, not to the user: they are written by the
	// server.
	Description string
	URI         string
	// Status is the HTTP status of the response, 0 for an error that came
	// in a redirect.
	Status int
}

func (e *Error) Error() string {
	s := "oauth: the server answered " + e.Code
	if e.Description != "" {
		s += ": " + e.Description
	}
	if e.Status != 0 {
		s += fmt.Sprintf(" (HTTP %d)", e.Status)
	}
	return s
}

// Is matches [ErrAccessDenied] and [ErrExpired] by code.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrAccessDenied:
		return e.Code == "access_denied"
	case ErrExpired:
		return e.Code == "expired_token"
	}
	return false
}
