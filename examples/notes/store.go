package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the pure-Go SQLite driver ("sqlite"); this module is the only one that has it
)

// The library lives in a SQLite database, notes.db in desktop.DataDir
// (docs/wiki/SQLite.md walks through this file):
//
//   - modernc.org/sqlite is SQLite translated to Go, so nothing needs cgo;
//   - the database is in WAL mode, so a reader never waits for the writer
//     and a write is one appended page, which is what typing makes;
//   - the schema has a version (PRAGMA user_version) and migrations that run
//     in one transaction each, so a database of an older Notes is upgraded
//     when it is opened and one of a newer Notes is refused;
//   - nothing here runs on the UI thread. A screen loads the library with
//     tela.UseAsync, and writes go through [store.Put], which keeps only the
//     latest library and returns at once: a goroutine writes what changed.

// dbName is the file of the library; legacyName is the JSON file of Notes
// 1.x, which is imported once.
const (
	dbName     = "notes.db"
	legacyName = "notes.json"
)

// migrations are the versions of the schema, in order: the database is at
// version len(migrations) when it is up to date. Never edit one that has
// shipped: add the next.
var migrations = []string{
	// 1: the library of Notes 2.
	`CREATE TABLE notes (
		id       INTEGER PRIMARY KEY,
		title    TEXT    NOT NULL,
		body     TEXT    NOT NULL,
		saved    TEXT    NOT NULL,
		path     TEXT    NOT NULL,
		folder   TEXT    NOT NULL,
		pinned   INTEGER NOT NULL,
		modified INTEGER NOT NULL, -- Unix nanoseconds, 0 for none
		locked   INTEGER NOT NULL,
		sealed   TEXT    NOT NULL
	);
	CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	// 2: sync (sync.go): the name of a note on the server, when it last
	// changed and was last pushed, and the notes that were deleted here
	// and not yet on the server.
	`ALTER TABLE notes ADD COLUMN uid     TEXT    NOT NULL DEFAULT '';
	ALTER TABLE notes ADD COLUMN updated INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE notes ADD COLUMN pushed  INTEGER NOT NULL DEFAULT 0;
	CREATE TABLE tombstones (uid TEXT PRIMARY KEY, deleted INTEGER NOT NULL);`,
}

// Tombstone is a note that was deleted here: the server learns it at the
// next sync, which needs the UID and when it happened.
type Tombstone struct {
	UID string
	At  time.Time
}

// snapshot is what the store keeps: the notes, the tombstones and the note
// that is open.
type snapshot struct {
	Notes    []Note
	Tombs    []Tombstone
	Selected int
}

// store is the library on disk. Opening the same path twice gives the same
// store, so two parts of a program never overwrite each other.
type store struct {
	path string // "" for an in-memory store (tela snapshot)
	dir  string
	refs int // open handles, under registry.mu

	// db is opened by ensure: at openStore, or for a lazyStore by the first
	// use, which is on the goroutine of a write or a load.
	openMu sync.Mutex
	db     *sql.DB

	mu      sync.Mutex
	cond    *sync.Cond
	queued  *snapshot
	writing bool
	written snapshot // what the database has
	known   bool     // written is what the database has (after Load or a write)
	onError func(error)
}

var registry struct {
	mu     sync.Mutex
	stores map[string]*store
}

// openStore opens (and creates) the library in dir. It reads and writes
// files: call it off the UI thread.
func openStore(dir string) (*store, error) {
	s, err := acquire(dir)
	if err != nil {
		return nil, err
	}
	if err := s.ensure(); err != nil {
		s.release()
		return nil, err
	}
	return s, nil
}

// lazyStore is the store of a library that does not exist yet (the first
// run): it touches no file until the first write, which makes it. That is
// what lets the first screen show without waiting for the disk.
func lazyStore(dir string) (*store, error) { return acquire(dir) }

// acquire returns the store of dir, making it when no handle has it. It does
// no file I/O.
func acquire(dir string) (*store, error) {
	path, err := filepath.Abs(filepath.Join(dir, dbName))
	if err != nil {
		return nil, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if s := registry.stores[path]; s != nil {
		s.refs++
		return s, nil
	}
	s := &store{path: path, dir: dir, refs: 1}
	s.cond = sync.NewCond(&s.mu)
	if registry.stores == nil {
		registry.stores = map[string]*store{}
	}
	registry.stores[path] = s
	return s, nil
}

// release gives a handle back, and forgets the store when it was the last.
func (s *store) release() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if s.refs--; s.refs <= 0 && s.path != "" {
		delete(registry.stores, s.path)
	}
}

// ensure opens the database and brings its schema up to date, once.
func (s *store) ensure() error {
	s.openMu.Lock()
	defer s.openMu.Unlock()
	if s.db != nil {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	// _pragma runs for every connection of the pool: WAL (the mode sticks to
	// the file), a wait of five seconds for a lock another connection holds,
	// synchronous=NORMAL, which WAL makes safe against a crash of the
	// program (only a power loss can lose the last write, never corrupt),
	// and secure_delete, which zeroes what a delete or an update leaves
	// behind in a page: a locked note must not keep its old text on disk.
	db, err := sql.Open("sqlite", s.path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=secure_delete(1)")
	if err != nil {
		return err
	}
	s.db = db
	if err := s.migrate(); err != nil {
		s.db = nil
		db.Close()
		return err
	}
	return nil
}

// memoryStore is a library that lives in memory, for the screens that
// `tela snapshot` renders: it touches no folder.
func memoryStore() *store {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err) // the driver is compiled in
	}
	db.SetMaxOpenConns(1) // every connection of ":memory:" is a database of its own
	s := &store{db: db, refs: 1}
	s.cond = sync.NewCond(&s.mu)
	if err := s.migrate(); err != nil {
		panic(err)
	}
	return s
}

// errNewerSchema is the error of a database made by a newer Notes.
var errNewerSchema = errors.New("the library was made by a newer version of Notes")

// migrate brings the schema to the latest version, one transaction per step.
func (s *store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("%w (schema %d, this one knows %d)", errNewerSchema, v, len(migrations))
	}
	for ; v < len(migrations); v++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		// PRAGMA takes no parameters; v is ours.
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec("PRAGMA user_version = " + strconv.Itoa(v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Close writes what is queued and closes the database when the last handle
// closes.
func (s *store) Close() error {
	s.Flush()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if s.refs--; s.refs > 0 {
		return nil
	}
	if s.path != "" {
		delete(registry.stores, s.path)
	}
	s.openMu.Lock()
	defer s.openMu.Unlock()
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// flushStores waits until every open library has written what is queued.
func flushStores() {
	registry.mu.Lock()
	all := make([]*store, 0, len(registry.stores))
	for _, s := range registry.stores {
		all = append(all, s)
	}
	registry.mu.Unlock()
	for _, s := range all {
		s.Flush()
	}
}

// closeStores flushes and closes every open library: main calls it when the
// app returns.
func closeStores() {
	flushStores()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for p, s := range registry.stores {
		s.openMu.Lock()
		if s.db != nil {
			s.db.Close()
		}
		s.openMu.Unlock()
		delete(registry.stores, p)
	}
}

// loaded is what Load returns.
type loaded struct {
	snapshot
	// Fresh is true for a library that never held anything: the first run.
	Fresh bool
	// Imported is how many notes came from notes.json.
	Imported int
	// Corrupt is true when notes.json could not be read: it was kept as
	// notes.json.corrupt.
	Corrupt bool
}

// Load reads the library, importing notes.json once when there is no
// library yet. It waits for the writes that are queued, so a second window
// of the app sees what the first one wrote.
func (s *store) Load() (loaded, error) {
	s.Flush()
	var out loaded
	if err := s.ensure(); err != nil {
		return out, err
	}
	imported, corrupt, err := s.importLegacy()
	if err != nil {
		return out, err
	}
	out.Imported, out.Corrupt = imported, corrupt
	snap, err := s.read()
	if err != nil {
		return out, err
	}
	out.snapshot = snap
	var initialized string
	err = s.db.QueryRow(`SELECT value FROM meta WHERE key = 'initialized'`).Scan(&initialized)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	out.Fresh = initialized == "" && len(snap.Notes) == 0
	return out, nil
}

// read selects the whole library.
func (s *store) read() (snapshot, error) {
	var snap snapshot
	rows, err := s.db.Query(`SELECT id, title, body, saved, path, folder, pinned, modified, locked, sealed, uid, updated, pushed
		FROM notes ORDER BY id`)
	if err != nil {
		return snap, err
	}
	defer rows.Close()
	var nameless []int
	for rows.Next() {
		var n Note
		var pinned, locked bool
		var modified, updated, pushed int64
		if err := rows.Scan(&n.ID, &n.Title, &n.Text, &n.Saved, &n.Path, &n.Folder, &pinned, &modified, &locked, &n.Sealed, &n.UID, &updated, &pushed); err != nil {
			return snap, err
		}
		n.Pinned, n.Locked = pinned, locked
		n.Modified, n.Updated, n.Pushed = fromNanos(modified), fromNanos(updated), fromNanos(pushed)
		// A note of a library imported from JSON has neither: it gets them
		// now, and the database keeps them.
		if n.UID == "" || n.Updated.IsZero() {
			if n.UID == "" {
				n.UID = newUID()
			}
			if n.Updated = n.Modified; n.Updated.IsZero() {
				n.Updated = now()
			}
			nameless = append(nameless, len(snap.Notes))
		}
		snap.Notes = append(snap.Notes, n)
	}
	if err := rows.Err(); err != nil {
		return snap, err
	}
	rows.Close()
	for _, i := range nameless {
		n := snap.Notes[i]
		if _, err := s.db.Exec(`UPDATE notes SET uid = ?, updated = ? WHERE id = ?`, n.UID, nanos(n.Updated), n.ID); err != nil {
			return snap, err
		}
	}
	trows, err := s.db.Query(`SELECT uid, deleted FROM tombstones ORDER BY uid`)
	if err != nil {
		return snap, err
	}
	defer trows.Close()
	for trows.Next() {
		var t Tombstone
		var at int64
		if err := trows.Scan(&t.UID, &at); err != nil {
			return snap, err
		}
		t.At = fromNanos(at)
		snap.Tombs = append(snap.Tombs, t)
	}
	if err := trows.Err(); err != nil {
		return snap, err
	}
	var sel string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'selected'`).Scan(&sel); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return snap, err
	}
	snap.Selected, _ = strconv.Atoi(sel)
	s.mu.Lock()
	s.written, s.known = cloneSnapshot(snap), true
	s.mu.Unlock()
	return snap, nil
}

// importLegacy reads the notes.json of Notes 1.x, once: the file is read
// when the database has no notes and was never initialized, and set aside as
// notes.json.imported afterwards. A file that does not parse is set aside as
// notes.json.corrupt, as the old store did, and the library starts empty.
func (s *store) importLegacy() (imported int, corrupt bool, err error) {
	if s.path == "" {
		return 0, false, nil
	}
	file := filepath.Join(filepath.Dir(s.path), legacyName)
	data, err := os.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return 0, false, nil
	case err != nil:
		return 0, false, err
	}
	var have int
	if err := s.db.QueryRow(`SELECT count(*) FROM notes`).Scan(&have); err != nil {
		return 0, false, err
	}
	var flag string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'initialized'`).Scan(&flag); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	if have > 0 || flag != "" {
		return 0, false, nil // the library is in use: the old file is not ours to import
	}
	var old struct {
		Notes    []Note `json:"notes"`
		Selected int    `json:"selected"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return 0, true, os.Rename(file, file+".corrupt")
	}
	snap := snapshot{Notes: old.Notes, Selected: old.Selected}
	if err := s.write(snapshot{}, snap); err != nil {
		return 0, false, err
	}
	if err := os.Rename(file, file+".imported"); err != nil {
		return 0, false, err
	}
	return len(old.Notes), false, nil
}

// Put queues the library for writing and returns at once. A later Put
// replaces one that was not written yet, so a burst of keystrokes is one
// transaction of the notes that changed.
func (s *store) Put(snap snapshot) {
	snap = cloneSnapshot(snap)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queued = &snap
	if !s.writing {
		s.writing = true
		go s.drain()
	}
}

// OnError sets the function called, on the writer's goroutine, when a write
// fails. The next Put tries again with everything that was not written.
func (s *store) OnError(f func(error)) {
	s.mu.Lock()
	s.onError = f
	s.mu.Unlock()
}

// drain writes the queued library until none is left.
func (s *store) drain() {
	for {
		s.mu.Lock()
		snap := s.queued
		s.queued = nil
		if snap == nil {
			s.writing = false
			s.cond.Broadcast()
			s.mu.Unlock()
			return
		}
		old, known := s.written, s.known
		s.mu.Unlock()
		if !known {
			// Nothing was loaded (a library that never opened): write all.
			old = snapshot{}
		}
		err := s.write(old, *snap)
		s.mu.Lock()
		if err == nil {
			s.written, s.known = *snap, true
		}
		fail := s.onError
		s.mu.Unlock()
		if err != nil && fail != nil {
			fail(err)
		}
	}
}

// Flush waits until everything queued is written.
func (s *store) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.queued != nil || s.writing {
		s.cond.Wait()
	}
}

// write makes the database hold next, changing only what differs from prev
// (what it held), in one transaction.
func (s *store) write(prev, next snapshot) error {
	if err := s.ensure(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old := make(map[int]Note, len(prev.Notes))
	for _, n := range prev.Notes {
		old[n.ID] = n
	}
	keep := make(map[int]bool, len(next.Notes))
	up, err := tx.Prepare(`INSERT INTO notes (id, title, body, saved, path, folder, pinned, modified, locked, sealed, uid, updated, pushed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title = excluded.title, body = excluded.body, saved = excluded.saved, path = excluded.path,
			folder = excluded.folder, pinned = excluded.pinned, modified = excluded.modified, locked = excluded.locked,
			sealed = excluded.sealed, uid = excluded.uid, updated = excluded.updated, pushed = excluded.pushed`)
	if err != nil {
		return err
	}
	defer up.Close()
	for _, n := range next.Notes {
		keep[n.ID] = true
		n.Unlocked = false
		if o, ok := old[n.ID]; ok && o == n {
			continue
		}
		if _, err := up.Exec(n.ID, n.Title, n.Text, n.Saved, n.Path, n.Folder, n.Pinned, nanos(n.Modified), n.Locked, n.Sealed,
			n.UID, nanos(n.Updated), nanos(n.Pushed)); err != nil {
			return err
		}
	}
	for id := range old {
		if !keep[id] {
			if _, err := tx.Exec(`DELETE FROM notes WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	if !slices.Equal(prev.Tombs, next.Tombs) {
		if _, err := tx.Exec(`DELETE FROM tombstones`); err != nil {
			return err
		}
		for _, t := range next.Tombs {
			if _, err := tx.Exec(`INSERT INTO tombstones (uid, deleted) VALUES (?, ?)`, t.UID, nanos(t.At)); err != nil {
				return err
			}
		}
	}
	if prev.Selected != next.Selected || len(prev.Notes) == 0 {
		if err := setMeta(tx, "selected", strconv.Itoa(next.Selected)); err != nil {
			return err
		}
	}
	if err := setMeta(tx, "initialized", "1"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if slices.ContainsFunc(next.Notes, func(n Note) bool { return n.Locked && old[n.ID] != n }) {
		// A note was locked: the page images of its text, from before, are
		// still in the write-ahead log until a checkpoint folds it into the
		// file and truncates it. (Not an error if a reader holds it up.)
		s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	return nil
}

func setMeta(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func cloneSnapshot(s snapshot) snapshot {
	s.Notes, s.Tombs = slices.Clone(s.Notes), slices.Clone(s.Tombs)
	return s
}

// nanos and fromNanos store a time as Unix nanoseconds, 0 for none.
func nanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func fromNanos(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// isCorruption reports whether err says the file is not a database, or is a
// damaged one.
func isCorruption(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "file is not a database") || strings.Contains(msg, "database disk image is malformed")
}

// setAside moves an unreadable library out of the way, with its WAL files,
// as notes.db.corrupt, so the next open starts a new one and nothing the
// user had is lost.
func setAside(dir string) error {
	file := filepath.Join(dir, dbName)
	for _, suffix := range []string{"-wal", "-shm"} {
		os.Remove(file + suffix)
	}
	return os.Rename(file, file+".corrupt")
}
