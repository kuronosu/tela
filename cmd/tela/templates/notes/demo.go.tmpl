package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/oauth/oauthtest"
)

// The demo server is what Notes talks to when no server is configured, so
// that `go run .` in examples/notes shows sign-in and sync on one machine,
// offline. It is a stand-in, not a product: one account, notes kept in
// memory (they are gone when the program ends, and the sign-in with them),
// and its OAuth half is package oauthtest's fake server, which approves a
// sign-in at once, behind the same address as the notes API. A real server
// is given with -server (or NOTES_SERVER): it must serve OAuth at
// /authorize, /token and /revoke and the API of sync.go at /api, for a
// client of the ID -client-id (NOTES_CLIENT_ID, "tela-notes" by default).

// serverInfo says where the sync server is.
type serverInfo struct {
	// URL is the base address, "http://127.0.0.1:port" for the demo server.
	URL string
	// ClientID is the OAuth client ID of Notes at that server.
	ClientID string
}

// demoServer is one running demo server.
type demoServer struct {
	// URL is the base address; Info is what the app needs to use it.
	URL  string
	Info serverInfo

	oauth *oauthtest.Server
	http  *http.Server
	hc    *http.Client

	down, rejecting atomic.Bool

	mu    sync.Mutex
	notes map[string]wireNote
}

// startDemo starts the demo server on a free port of the loopback
// interface. Close it when done.
func startDemo() (*demoServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	d := &demoServer{
		oauth: oauthtest.NewServer(),
		hc:    &http.Client{Timeout: 10 * time.Second},
		notes: map[string]wireNote{},
	}
	d.URL = "http://" + ln.Addr().String()
	d.Info = serverInfo{URL: d.URL, ClientID: oauthtest.ClientID}
	target, _ := url.Parse(d.oauth.URL)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", d.auth(d.handleMe))
	mux.HandleFunc("/api/notes", d.auth(d.handleList))
	mux.HandleFunc("/api/notes/", d.auth(d.handlePut))
	mux.Handle("/", httputil.NewSingleHostReverseProxy(target)) // authorize, token, revoke
	d.http = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go d.http.Serve(ln)
	return d, nil
}

// Close stops the server.
func (d *demoServer) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	d.http.Shutdown(ctx)
	d.oauth.Close()
}

// SetDown makes the API answer 503 (no network, for the tests of an app
// that works offline); SetRejecting makes it answer 401 to every token, as
// a server that forgot the sign-in does.
func (d *demoServer) SetDown(down bool)        { d.down.Store(down) }
func (d *demoServer) SetRejecting(reject bool) { d.rejecting.Store(reject) }

// OAuth is the fake OAuth server behind the address: a test makes it deny
// or expire.
func (d *demoServer) OAuth() *oauthtest.Server { return d.oauth }

// Notes returns the notes the server has, deleted ones included, by UID.
func (d *demoServer) Notes() []wireNote {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]wireNote, 0, len(d.notes))
	for _, n := range d.notes {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b wireNote) int { return strings.Compare(a.UID, b.UID) })
	return out
}

// Put stores n as another device would have, if it is newer than what the
// server has; it reports whether it was stored.
func (d *demoServer) Put(n wireNote) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if old, ok := d.notes[n.UID]; ok && !n.Updated.After(old.Updated) {
		return false
	}
	d.notes[n.UID] = n
	return true
}

// auth runs h for a request with a valid bearer token, which the fake OAuth
// server's protected resource says: the same server that issued it.
func (d *demoServer) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.down.Load() {
			http.Error(w, "the demo server is down", http.StatusServiceUnavailable)
			return
		}
		if d.rejecting.Load() {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, d.oauth.ResourceURL(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", r.Header.Get("Authorization"))
		resp, err := d.hc.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (d *demoServer) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"name": "Demo user"})
}

func (d *demoServer) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, d.Notes())
}

// handlePut stores PUT /api/notes/{uid}: last writer wins, by Updated.
func (d *demoServer) handlePut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "use PUT", http.StatusMethodNotAllowed)
		return
	}
	uid, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/notes/"))
	var n wireNote
	if err != nil || uid == "" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&n) != nil || n.UID != uid || n.Updated.IsZero() {
		http.Error(w, fmt.Sprintf("the body must be a note with uid %q and an updated time", uid), http.StatusBadRequest)
		return
	}
	d.Put(n)
	d.mu.Lock()
	stored := d.notes[uid]
	d.mu.Unlock()
	writeJSON(w, stored)
}
