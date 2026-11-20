package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// DeviceCode is what the user needs to approve a device sign-in (RFC 8628
// §3.2): show the code and where to type it, or the complete URL as a link
// or a QR code.
type DeviceCode struct {
	// UserCode is the short code to type at VerificationURI.
	UserCode string
	// VerificationURI is where the user types the code.
	VerificationURI string
	// VerificationURIComplete has the code in it; empty when the server
	// gives none.
	VerificationURIComplete string
	// ExpiresIn is how long the code is valid from when it was issued.
	ExpiresIn time.Duration
	// Interval is the time between the app's polls.
	Interval time.Duration
}

// deviceResponse is the JSON of the device authorization endpoint.
type deviceResponse struct {
	DeviceCode              string      `json:"device_code"`
	UserCode                string      `json:"user_code"`
	VerificationURI         string      `json:"verification_uri"`
	VerificationURL         string      `json:"verification_url"` // Google's early spelling
	VerificationURIComplete string      `json:"verification_uri_complete"`
	ExpiresIn               flexSeconds `json:"expires_in"`
	Interval                flexSeconds `json:"interval"`
}

// Polling constants of RFC 8628 §3.2 and §3.5.
const (
	defaultInterval = 5 * time.Second
	defaultExpiry   = 15 * time.Minute
)

// slowDownStep is added to the interval at each slow_down (RFC 8628 §3.5).
var slowDownStep = 5 * time.Second

// DeviceSignIn signs the user in with the device authorization flow (RFC
// 8628), for an app that has no browser of its own or whose user has a
// phone at hand. It asks the server for a code, calls show with it (from
// the calling goroutine, before polling starts: show it to the user, and
// do not block), and polls the token endpoint at the server's interval,
// slowing down when it says so, until the user approves, denies
// ([ErrAccessDenied]), the code expires ([ErrExpired]) or ctx is done.
// It needs Config.DeviceURL. Run it off the UI thread.
func (s *Session) DeviceSignIn(ctx context.Context, show func(DeviceCode)) error {
	cfg := &s.cfg
	if err := cfg.need("DeviceSignIn", "DeviceURL", cfg.DeviceURL); err != nil {
		return err
	}
	if err := cfg.need("DeviceSignIn", "TokenURL", cfg.TokenURL); err != nil {
		return err
	}
	form := url.Values{"client_id": {cfg.ClientID}}
	if len(cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	if cfg.ClientSecret != "" && cfg.SecretInBody {
		form.Set("client_secret", cfg.ClientSecret)
	}
	body, err := s.post(ctx, cfg.DeviceURL, form)
	if err != nil {
		return fmt.Errorf("oauth: asking for a device code: %w", err)
	}
	var r deviceResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("oauth: the device endpoint did not answer with JSON: %w", err)
	}
	if r.VerificationURI == "" {
		r.VerificationURI = r.VerificationURL
	}
	if r.DeviceCode == "" || r.UserCode == "" || r.VerificationURI == "" {
		return errors.New("oauth: the device endpoint's answer lacks device_code, user_code or verification_uri")
	}
	dc := DeviceCode{
		UserCode:                r.UserCode,
		VerificationURI:         r.VerificationURI,
		VerificationURIComplete: r.VerificationURIComplete,
		ExpiresIn:               time.Duration(float64(r.ExpiresIn) * float64(time.Second)),
		Interval:                time.Duration(float64(r.Interval) * float64(time.Second)),
	}
	if dc.ExpiresIn <= 0 {
		dc.ExpiresIn = defaultExpiry
	}
	if dc.Interval <= 0 {
		dc.Interval = defaultInterval
	}
	if show != nil {
		show(dc)
	}

	deadline := time.NewTimer(dc.ExpiresIn)
	defer deadline.Stop()
	interval := dc.Interval
	for {
		wait := time.NewTimer(interval)
		select {
		case <-wait.C:
		case <-deadline.C:
			wait.Stop()
			return fmt.Errorf("oauth: the device code expired before it was approved: %w", ErrExpired)
		case <-ctx.Done():
			wait.Stop()
			return ctx.Err()
		}
		tok, err := s.tokenRequest(ctx, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {r.DeviceCode},
		})
		if err == nil {
			return s.signedIn(tok)
		}
		var oe *Error
		if !errors.As(err, &oe) {
			return fmt.Errorf("oauth: polling for the approval: %w", err)
		}
		switch oe.Code {
		case "authorization_pending":
		case "slow_down":
			interval += slowDownStep
		default: // access_denied, expired_token, invalid_grant, …
			return fmt.Errorf("oauth: the device sign-in ended: %w", err)
		}
	}
}
