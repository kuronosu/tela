package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"

	"github.com/kuronosu/tela/desktop"
)

// Update is a newer release offered to this install, returned by [Check].
type Update struct {
	// Release is the release as the manifest lists it: Version, Date,
	// Notes and the rest.
	Release
	// Artifact is the file this install downloads.
	Artifact Artifact
	// Policy is when the update takes effect: Config.Policy, else the
	// release's activation, else ActivateNextStart.
	Policy Activation
	// From is the running version.
	From string

	cfg  Config
	inst *install
	url  string // the artifact's resolved URL
	file string // the verified download, after Download
}

// Check fetches the manifest at cfg.URL and its signature, and returns the
// release this install should update to.
//
// Before reading anything in the manifest it verifies the signature over
// the manifest's exact bytes against cfg.PublicKeys ([ErrSignature]).
// It then refuses a manifest of another app ID (desktop.AppID), one
// past its expiry ([ErrExpired]) and one published before the newest one
// this install accepted ([ErrOlderManifest]), and records the time of the
// one it accepts in desktop.StateDir.
//
// The running version (the one tela build or tela package wrote into the
// binary, ADR-294) below the manifest's endOfSupport is an
// [*EndOfSupportError]. Otherwise the release is the newest one of
// cfg.Channel that is newer than the running version, includes this
// install in its rollout, accepts the running version (minVersion), was
// not rolled back on this install, and has an artifact for the running
// GOOS, GOARCH and install format; none is [ErrUpToDate].
//
// Rollout: on its first Check an install makes a random ID and keeps it
// in desktop.StateDir. Its bucket for a version is the first 8 bytes of
// SHA-256(ID, a zero byte, version) as a big-endian number scaled to
// [0, 1); the release is offered when the bucket is below its rollout.
// The same install gets the same answer for the same manifest, and a
// raise of the rollout only adds installs.
//
// Installs that a package manager updates return a [*ManagedError] at
// once, without a request: deb (apt), flatpak, MSIX (its .appinstaller).
// A program that is not an install (go run, tela build) returns
// [ErrNotInstalled], and a binary without a version [ErrNoVersion].
func Check(ctx context.Context, cfg Config) (*Update, error) {
	if cfg.URL == "" {
		return nil, errors.New("update.Check: Config.URL is empty")
	}
	inst, err := findInstall(cfg.Format)
	if err != nil {
		return nil, err
	}
	cur := currentVersion()
	if cur == "" {
		return nil, ErrNoVersion
	}
	curV, err := parseSemver(cur)
	if err != nil {
		return nil, fmt.Errorf("update: the running version: %w", err)
	}
	m, err := fetchManifest(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if id := desktop.AppID(); m.AppID != id {
		return nil, fmt.Errorf("update: the manifest is for app %q, and this app is %q", m.AppID, id)
	}
	if t := now(); !t.Before(m.Expires) {
		return nil, fmt.Errorf("%w (on %s)", ErrExpired, m.Expires.UTC().Format("2006-01-02 15:04 MST"))
	}

	stateMu.Lock()
	defer stateMu.Unlock()
	us, path, err := loadUserState()
	if err != nil {
		return nil, err
	}
	if m.Published.Before(us.Newest) {
		return nil, fmt.Errorf("%w: published %s, and %s was seen", ErrOlderManifest, m.Published.UTC().Format("2006-01-02T15:04:05Z"), us.Newest.UTC().Format("2006-01-02T15:04:05Z"))
	}
	if m.Published.After(us.Newest) {
		us.Newest = m.Published
		if err := writeJSON(path, us); err != nil {
			return nil, err
		}
	}

	if eos := m.EndOfSupport; eos != nil && eos.Below != "" {
		below, err := parseSemver(eos.Below)
		if err != nil {
			return nil, fmt.Errorf("update: manifest endOfSupport: %w", err)
		}
		if curV.compare(below) < 0 {
			return nil, &EndOfSupportError{Version: cur, Message: eos.Message, URL: eos.URL}
		}
	}

	ist, err := inst.loadState()
	if err != nil {
		return nil, err
	}
	t := target{os: runtime.GOOS, arch: runtime.GOARCH, format: inst.format, current: curV, installID: us.InstallID}
	for _, rb := range ist.Rollbacks {
		t.skip = append(t.skip, rb.Version)
	}
	// A version applied but not started yet is installed already.
	if p := ist.Pending; p != nil && p.From == cur {
		if pv, err := parseSemver(p.Version); err == nil && pv.compare(curV) > 0 {
			t.current = pv
		}
	}
	r, a, err := choose(m.Channels[cfg.channel()], t)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrUpToDate
	}
	u, err := resolveURL(cfg.URL, a.URL)
	if err != nil {
		return nil, err
	}
	policy := cfg.Policy
	if policy == "" {
		policy = r.Activation
	}
	return &Update{Release: *r, Artifact: *a, Policy: policy.known(), From: cur, cfg: cfg, inst: inst, url: u}, nil
}

// fetchManifest fetches the manifest and its signature, verifies it and
// parses it.
func fetchManifest(ctx context.Context, cfg Config) (*Manifest, error) {
	sigURL, err := signatureURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	data, err := fetch(ctx, cfg.client(), cfg.URL, maxManifest)
	if err != nil {
		return nil, err
	}
	sig, err := fetch(ctx, cfg.client(), sigURL, maxSignature)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSignature, err)
	}
	if err := Verify(data, sig, cfg.PublicKeys); err != nil {
		return nil, err
	}
	return parseManifest(data)
}

// signatureURL returns the URL of the signature of the manifest at
// manifestURL: its path with ".sig" added, the query kept.
func signatureURL(manifestURL string) (string, error) {
	u, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("update: Config.URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("update: Config.URL %q: only http and https are supported", manifestURL)
	}
	u.Path += ".sig"
	if u.RawPath != "" {
		u.RawPath += ".sig"
	}
	return u.String(), nil
}

// fetch GETs url and returns its body, at most limit bytes.
func fetch(ctx context.Context, c *http.Client, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: GET %s: %s", u, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("update: GET %s: %w", u, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("update: GET %s: larger than %d bytes", u, limit)
	}
	return data, nil
}
