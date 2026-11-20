package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

func testStore(t *testing.T) (*store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, dir
}

// The database is in WAL mode, has the latest schema, and gives back what
// was put in it.
func TestStoreRoundTrip(t *testing.T) {
	st, dir := testStore(t)
	var mode string
	if err := st.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v, want wal", mode, err)
	}
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != len(migrations) {
		t.Errorf("user_version = %d, %v, want %d", version, err, len(migrations))
	}
	at := time.Date(2026, time.October, 30, 9, 41, 0, 123, time.UTC)
	notes := []Note{
		{ID: 1, Title: "A", Text: "one", Saved: "one", UID: "u1", Modified: at, Updated: at, Pushed: at, Pinned: true, Folder: "Home", Path: "/x/a.md"},
		{ID: 7, Title: "B", Text: "two", UID: "u7", Updated: at.Add(time.Second), Locked: true, Sealed: "v1:xx"},
	}
	tombs := []Tombstone{{UID: "gone", At: at}}
	st.Put(snapshot{Notes: notes, Tombs: tombs, Selected: 7})
	st.Flush()

	got, err := st.read()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 2 || got.Notes[0] != notes[0] || got.Notes[1] != notes[1] || got.Selected != 7 || len(got.Tombs) != 1 || got.Tombs[0] != tombs[0] {
		t.Errorf("read = %+v\nwant notes %+v, tombs %+v, selected 7", got, notes, tombs)
	}

	// A change writes only what differs, and a deletion removes the row.
	notes[0].Text = "one, edited"
	st.Put(snapshot{Notes: notes[:1], Selected: 1})
	st.Close()
	st2, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	ld, err := st2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(ld.Notes) != 1 || ld.Notes[0].Text != "one, edited" || ld.Selected != 1 || len(ld.Tombs) != 0 || ld.Fresh {
		t.Errorf("after reopening: %+v", ld)
	}
}

// Put returns at once even while the database is locked by another
// writer: typing never waits for the disk. What was put is written when the
// lock goes.
func TestStorePutDoesNotWait(t *testing.T) {
	st, dir := testStore(t)
	st.Put(snapshot{Notes: []Note{{ID: 1, Title: "first", UID: "u1", Updated: fixedNow}}})
	st.Flush()

	// Another connection takes the write lock and keeps it.
	other, err := sql.Open("sqlite", filepath.Join(dir, dbName)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := other.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE notes SET title = title`); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	for i := 0; i < 50; i++ {
		st.Put(snapshot{Notes: []Note{{ID: 1, Title: "first", Text: string(rune('a' + i%26)), UID: "u1", Updated: fixedNow}}})
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("50 Puts took %v while the database was locked, want them not to wait", d)
	}
	tx.Rollback() // the lock goes: the writer's busy wait ends
	st.Flush()
	got, err := st.read()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 1 || got.Notes[0].Text != string(rune('a'+49%26)) {
		t.Errorf("after the lock: %+v, want the last text put", got.Notes)
	}
}

// A write that fails is reported (the screen shows a toast).
func TestStoreWriteErrorIsReported(t *testing.T) {
	st, _ := testStore(t)
	failed := make(chan error, 4)
	st.OnError(func(err error) { failed <- err })
	st.db.Close() // every write fails now
	st.Put(snapshot{Notes: []Note{{ID: 1, Title: "x", UID: "u"}}})
	st.Flush()
	select {
	case err := <-failed:
		if err == nil {
			t.Error("the error is nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a failed write was not reported")
	}
}

// A database of the first schema is migrated when it is opened: the notes
// get their UID and Updated time, and nothing is lost.
func TestStoreMigratesAnOlderSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, dbName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	at := fixedNow.UnixNano()
	if _, err := db.Exec(`INSERT INTO notes VALUES (1, 'Old', 'body', 'body', '', '', 0, ?, 0, '')`, at); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	setClock(t, fixedNow.Add(time.Hour))
	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ld, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(ld.Notes) != 1 || ld.Notes[0].Title != "Old" || ld.Notes[0].UID == "" || !ld.Notes[0].Updated.Equal(fixedNow) {
		t.Errorf("the migrated library = %+v, want the note with a UID and its modified time as Updated", ld.Notes)
	}
	var v int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != len(migrations) {
		t.Errorf("user_version = %d, %v, want %d", v, err, len(migrations))
	}
}

// The notes.json of Notes 1.x is imported once, with the note that was open,
// and set aside.
func TestImportsNotesJSON(t *testing.T) {
	sandbox(t)
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	old := `{"notes":[
		{"id":1,"title":"Plain","text":"a","saved":"a","path":"/home/ana/plain.md","folder":"Home","pinned":true},
		{"id":2,"text":"Groceries\nmilk","saved":""}],
		"selected":2}`
	if err := os.WriteFile(filepath.Join(dir, legacyName), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	a := mount(t)
	a.ExpectValue(editor, "Groceries\nmilk") // the note that was open
	a.ExpectRole("button", "Plain")
	a.ExpectCount(telatest.ByRole("button", "Welcome to Notes"), 0) // the welcome notes are for the first run
	a.Close()

	if _, err := os.Stat(filepath.Join(dir, legacyName)); err == nil {
		t.Error("notes.json is still there after the import")
	}
	if _, err := os.Stat(filepath.Join(dir, legacyName+".imported")); err != nil {
		t.Errorf("the imported file was not kept: %v", err)
	}
	notes := storedNotes(t)
	if len(notes) != 2 || notes[0].Folder != "Home" || !notes[0].Pinned || notes[0].Path != "/home/ana/plain.md" || notes[0].UID == "" || notes[1].UID == "" {
		t.Errorf("the imported notes = %+v", notes)
	}

	// Once: a notes.json that shows up later is not imported again.
	if err := os.WriteFile(filepath.Join(dir, legacyName), []byte(`{"notes":[{"id":9,"text":"later"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b := mount(t)
	b.ExpectRole("button", "Plain")
	b.ExpectNoText("later")
	if _, ok := find(storedNotes(t), 9); ok {
		t.Error("a second notes.json was imported")
	}
}
