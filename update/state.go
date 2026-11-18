package update

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/appver"
)

// Replaced in tests.
var (
	now            = time.Now
	currentVersion = appver.Version
)

// stateMu serializes the reads and writes of the state files within the
// process. Two processes of the same app checking at once may race; the
// worst outcome is a manifest time not recorded.
var stateMu sync.Mutex

// userState is the update state of the app for the user, in
// <desktop.StateDir>/update/state.json.
type userState struct {
	// InstallID is a random 128-bit ID, in hex, made on the first Check:
	// the input of the rollout buckets. It identifies nothing else and
	// is never sent.
	InstallID string `json:"installID"`
	// Newest is the Published time of the newest manifest accepted.
	Newest time.Time `json:"newestManifest,omitzero"`
}

func userStatePath() (string, error) {
	dir, err := desktop.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update", "state.json"), nil
}

// loadUserState returns the user state, making the install ID when it is
// missing. Call it with stateMu held.
func loadUserState() (*userState, string, error) {
	path, err := userStatePath()
	if err != nil {
		return nil, "", err
	}
	var st userState
	if err := readJSON(path, &st); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	if st.InstallID == "" {
		var b [16]byte
		_, _ = rand.Read(b[:])
		st.InstallID = hex.EncodeToString(b[:])
		if err := writeJSON(path, &st); err != nil {
			return nil, "", err
		}
	}
	return &st, path, nil
}

// installState is the swap state of an install, in its update folder
// (install.updateDir), so it does not depend on the app ID and is read
// before anything else starts.
type installState struct {
	// Pending is the version Apply put in place that has not been marked
	// healthy, nil when there is none.
	Pending *pending `json:"pending,omitempty"`
	// Rollbacks are the versions put back on this install, newest last.
	// Check does not offer them again.
	Rollbacks []Rollback `json:"rollbacks,omitempty"`
}

// pending is an applied, unconfirmed version.
type pending struct {
	Version string    `json:"version"`
	From    string    `json:"from"`
	Applied time.Time `json:"applied"`
	// Starts counts the starts of Version since Apply.
	Starts int `json:"starts"`
	// Files and OldFiles are the zip folder's entries put in place and
	// moved to the old copy.
	Files    []string `json:"files,omitempty"`
	OldFiles []string `json:"oldFiles,omitempty"`
}

// Rollback records a version that was put back.
type Rollback struct {
	// Version is the version taken out, and From the one put back.
	Version string    `json:"version"`
	From    string    `json:"from"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

func (in *install) statePath() string { return filepath.Join(in.updateDir(), "state.json") }

func (in *install) loadState() (*installState, error) {
	var st installState
	if err := readJSON(in.statePath(), &st); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return &st, nil
}

func (in *install) saveState(st *installState) error {
	return writeJSON(in.statePath(), st)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("update: %s: %w", path, err)
	}
	return nil
}

// writeJSON writes v to path through a temporary file and a rename, so a
// crash leaves the old content or the new.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
