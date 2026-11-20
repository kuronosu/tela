package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

// --- The cipher ------------------------------------------------------

func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := newKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestSealRoundTrip(t *testing.T) {
	key := testKey(t)
	want := content{Title: "Diary", Text: "Dear diary,\nnobody reads this.", Saved: "Dear"}
	sealed, err := seal(key, 7, want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1:") {
		t.Errorf("sealed = %q, want the v1: prefix", sealed)
	}
	if strings.Contains(sealed, "diary") || strings.Contains(sealed, "Diary") {
		t.Errorf("sealed value has plain text: %q", sealed)
	}
	got, err := unseal(key, 7, sealed)
	if err != nil || got != want {
		t.Errorf("unseal = %+v, %v; want %+v", got, err, want)
	}
}

// Every write has a nonce of its own: the same content seals differently.
func TestSealUsesANewNonceEveryTime(t *testing.T) {
	key := testKey(t)
	c := content{Text: "same"}
	a, _ := seal(key, 1, c)
	b, _ := seal(key, 1, c)
	if a == b {
		t.Error("two seals of the same content are equal: the nonce is not random")
	}
}

func TestUnsealRefuses(t *testing.T) {
	key := testKey(t)
	sealed, _ := seal(key, 1, content{Text: "secret"})
	other := testKey(t)

	cases := []struct {
		name   string
		key    []byte
		id     int
		sealed string
	}{
		{"another key", other, 1, sealed},
		{"another note", key, 2, sealed}, // the ID is authenticated
		{"no prefix", key, 1, strings.TrimPrefix(sealed, "v1:")},
		{"not base64", key, 1, "v1:***"},
		{"too short", key, 1, "v1:AAAA"},
		{"short key", key[:16], 1, sealed},
		{"tampered", key, 1, sealed[:len(sealed)-6] + "AAAAAA"},
	}
	for _, c := range cases {
		if got, err := unseal(c.key, c.id, c.sealed); err == nil {
			t.Errorf("%s: unseal opened it: %+v", c.name, got)
		}
	}
}

func TestVault(t *testing.T) {
	var v vault
	key := testKey(t)
	locked := lockedForm(Note{ID: 3, Title: "T", Text: "body", Saved: "b", Folder: "Home", Pinned: true}, "v1:x")
	if locked.Title != "" || locked.Text != "" || locked.Saved != "" || !locked.Locked || locked.Folder != "Home" || !locked.Pinned {
		t.Errorf("lockedForm = %+v, want no content and the rest kept", locked)
	}
	if !locked.Hidden() || locked.Heading() != "Locked note" || locked.Tags() != nil {
		t.Errorf("a locked note = hidden %v, heading %q, tags %q", locked.Hidden(), locked.Heading(), locked.Tags())
	}

	v.unlock(3, key, content{Title: "T", Text: "body #tag", Saved: "b"})
	shown := v.view(locked)
	if shown.Hidden() || shown.Title != "T" || shown.Text != "body #tag" || !shown.Unlocked || shown.Heading() != "T" {
		t.Errorf("view of an unlocked note = %+v", shown)
	}
	if shown.Tags() != nil {
		t.Errorf("an unlocked locked note has tags %q; they would show its words in the rail", shown.Tags())
	}
	if matchesQuery(shown, "body") {
		t.Error("a search found text inside a locked note")
	}

	sealed, err := v.change(3, func(n *Note) { n.Text = "new text" })
	if err != nil {
		t.Fatal(err)
	}
	if c, err := unseal(key, 3, sealed); err != nil || c.Text != "new text" || c.Title != "T" {
		t.Errorf("change sealed %+v, %v", c, err)
	}

	v.lock(3)
	if v.key != nil || v.isOpen(3) || !v.view(locked).Hidden() {
		t.Errorf("after lock: key %v, open %v", v.key, v.isOpen(3))
	}
	// The key the vault zeroed was its own copy.
	if bytes.Equal(key, make([]byte, keySize)) {
		t.Error("locking zeroed the caller's key")
	}
}

// The new fields are absent from the file of a note that is not locked, so
// the library a note of the earlier versions wrote is written back as it was.
func TestLockFieldsAreOptional(t *testing.T) {
	data, err := json.Marshal(Note{ID: 1, Text: "a", Saved: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "locked") || strings.Contains(string(data), "sealed") || strings.Contains(string(data), "Unlocked") {
		t.Errorf("a note that is not locked is written with lock fields: %s", data)
	}
	var old Note
	if err := json.Unmarshal([]byte(`{"id":1,"title":"A","text":"b","saved":"b","path":"","folder":"","pinned":false,"modified":"0001-01-01T00:00:00Z"}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.Locked || old.Hidden() || old.Sealed != "" || old.Title != "A" {
		t.Errorf("an old note = %+v", old)
	}
}

// --- The screen ------------------------------------------------------

var (
	lockNote   = telatest.ByRole("button", "Lock note")
	lockNow    = telatest.ByRole("button", "Lock now")
	unlockBtn  = telatest.ByRole("button", "Unlock")
	lockedCard = telatest.ByRole("button", "Locked note, locked")
)

// libraryFile waits for the writes that are queued and returns the library
// as it is on disk: the bytes of the database and of its write-ahead log
// (SQLite keeps text in its pages as it is, so a word that is in the clear in
// the library is in these bytes).
func libraryFile(t *testing.T) string {
	t.Helper()
	flushStores()
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, name := range []string{dbName, dbName + "-wal"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			all.Write(data)
		}
	}
	return all.String()
}

// openedLibrary opens the library of the test's data folder as the app
// does (the same store while the app has it open), and gives it back
// closed with what it holds.
func storedLibrary(t *testing.T) snapshot {
	t.Helper()
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.Flush()
	snap, err := st.read()
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// storedNotes returns the notes of the library as the database has them.
func storedNotes(t *testing.T) []Note { return storedLibrary(t).Notes }

// sealedOf returns the sealed value of the stored note id.
func sealedOf(t *testing.T, id int) string {
	t.Helper()
	n, ok := find(storedNotes(t), id)
	if !ok {
		t.Fatalf("the library has no note %d", id)
	}
	return n.Sealed
}

// noPlaintext fails the test when the library says any of the words in the
// clear.
func noPlaintext(t *testing.T, words ...string) {
	t.Helper()
	file := libraryFile(t)
	for _, w := range words {
		if strings.Contains(file, w) {
			t.Errorf("the library has %q in the clear", w)
		}
	}
}

// noneLocked fails the test when the library has a locked note.
func noneLocked(t *testing.T) {
	t.Helper()
	for _, n := range storedNotes(t) {
		if n.Locked || n.Sealed != "" {
			t.Errorf("the library has a locked note: %+v", n)
		}
	}
}

// grocery is the id of the Groceries note of the welcome library.
const grocery = 3

// lockGroceries opens Groceries and locks it.
func lockGroceries(t *testing.T, a *telatest.App) {
	t.Helper()
	a.Click(card("Groceries"))
	a.ExpectValue(editor, groceriesText)
	a.Click(lockNote)
	a.ExpectText("This note is locked")
}

func TestLockEncryptsTheNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"))
	a.Click(lockNote)

	// The note is a locked card now, with its lock, and no text: the editor
	// says so, and offers to unlock it.
	a.ExpectText("This note is locked")
	a.ExpectCount(editor, 0)
	a.ExpectCount(card("Groceries"), 0)
	a.ExpectCount(lockedCard, 1)
	a.ExpectText("Encrypted in your library")
	a.ExpectRole("button", "Unlock")

	// A key was made, in the keyring: 32 bytes. The file has the ciphertext
	// and not a word of the note.
	key, ok := a.StoredSecret("library-key")
	if !ok || len(key) != 32 {
		t.Fatalf("the keyring has %d bytes (found %v), want a 32-byte key", len(key), ok)
	}
	noPlaintext(t, "Groceries", "Coffee", "oats", "errands")
	n, _ := find(storedNotes(t), grocery)
	if !n.Locked || !strings.HasPrefix(n.Sealed, "v1:") || n.Title != "" || n.Text != "" || n.Saved != "" || n.Folder != "Home" {
		t.Errorf("the stored note = %+v, want it locked and sealed, with no content and its folder", n)
	}
	// Its tag leaves the rail with its text.
	a.ExpectCount(telatest.ByRole("button", "#errands"), 0)
	a.ExpectCount(telatest.ByRole("button", "Groceries"), 0)
}

func TestUnlockShowsEditsAndSealsAgain(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	before := sealedOf(t, grocery)

	a.Click(unlockBtn)
	a.ExpectValue(titleBox, "Groceries")
	a.ExpectValue(editor, groceriesText)
	a.ExpectRole("button", "Groceries, locked") // the card has its title back while it is unlocked
	a.ExpectRole("button", "Lock now")
	if sealedOf(t, grocery) != before {
		t.Error("unlocking changed the library")
	}

	// An edit is encrypted at once: the file never has the words, and its
	// sealed value changes with a new nonce.
	a.Click(editor)
	a.Press("Control+End")
	a.Type(" Plus saffron.")
	a.ExpectValue(editor, groceriesText+" Plus saffron.")
	noPlaintext(t, "saffron", "Coffee", "Groceries")
	if sealedOf(t, grocery) == before {
		t.Error("an edit of an unlocked note was not stored")
	}

	// The card never shows a preview, unlocked or not, and a search does
	// not look inside it.
	a.ExpectNoText("saffron.\n")
	a.Click(telatest.ByRole("searchbox", "Search notes"))
	a.Type("saffron")
	a.ExpectText("Nothing matches")
	a.ExpectCount(telatest.ByRole("button", "Groceries, locked"), 0)
}

func TestLockAgainAndHideLockTheNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.Click(unlockBtn)
	a.ExpectValue(editor, groceriesText)

	// "Lock now" hides it again without asking the keyring.
	a.FailSecrets(desktop.ErrSecretCanceled)
	a.Click(lockNow)
	a.ExpectText("This note is locked")
	a.ExpectCount(editor, 0)
	a.FailSecrets(nil)

	// Hiding the window (the tray) locks every note again.
	a.Click(unlockBtn)
	a.ExpectValue(editor, groceriesText)
	a.ClickTrayItem("Hide Notes")
	a.ExpectWindowState(telatest.Hidden(true))
	a.ClickTrayItem("Show Notes")
	a.ExpectText("This note is locked")
	a.ExpectCount(editor, 0)
	noPlaintext(t, "Coffee")
}

func TestUnlockAfterARestart(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	key, _ := a.StoredSecret("library-key")
	a.Close()

	b := mount(t)
	b.SetStoredSecret("library-key", key) // the keyring of the same user
	b.ExpectRole("button", "Locked note, locked")
	b.Click(lockedCard)
	b.Click(unlockBtn)
	b.ExpectValue(editor, groceriesText)
	b.ExpectValue(titleBox, "Groceries")
}

// The user dismisses the keyring's prompt: a quiet toast and nothing else.
func TestUnlockCanceled(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)

	a.FailSecrets(desktop.ErrSecretCanceled)
	a.Click(unlockBtn)
	a.ExpectText("The keyring stayed locked")
	a.ExpectText("This note is locked")
	a.ExpectCount(editor, 0)
	a.ExpectNoText("Could not")
	a.ExpectRole("button", "Unlock") // and it can be tried again

	a.FailSecrets(nil)
	a.Click(unlockBtn)
	a.ExpectValue(editor, groceriesText)
}

func TestLockCanceled(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"))
	a.FailSecrets(desktop.ErrSecretCanceled)
	a.Click(lockNote)
	a.ExpectText("The keyring stayed locked")
	a.ExpectValue(editor, groceriesText) // not locked
	noneLocked(t)
	if _, ok := a.StoredSecret("library-key"); ok {
		t.Error("a key was stored although the prompt was canceled")
	}
}

// With locked notes and no key (the keyring was reset, or the library came
// from another computer) the data stays and the app says why.
func TestKeyIsGone(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	locked := sealedOf(t, grocery)
	a.Close()

	b := mount(t) // a keyring with no key
	b.Click(lockedCard)
	b.Click(unlockBtn)
	b.ExpectText("The key for your locked notes is gone")
	b.ExpectText("no longer in the system keyring")
	b.ExpectRole("button", "Try again")
	if _, ok := b.StoredSecret("library-key"); ok {
		t.Error("a new key was made although locked notes exist: it would not open them")
	}
	if sealedOf(t, grocery) != locked {
		t.Error("the encrypted note changed")
	}

	// A new lock does not make a key either.
	b.Click(card("Keyboard shortcuts"))
	b.Click(lockNote)
	b.ExpectValue(editor, shortcutsText("Ctrl"))
	if _, ok := b.StoredSecret("library-key"); ok {
		t.Error("a note was locked with a new key while the library has notes for the old one")
	}
}

// A key that is not ours, or a library that was altered, does not unlock
// a note, and nothing is lost.
func TestWrongKey(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	locked := sealedOf(t, grocery)
	a.Close()

	b := mount(t)
	b.SetStoredSecret("library-key", bytes.Repeat([]byte{7}, 32))
	b.Click(lockedCard)
	b.Click(unlockBtn)
	b.ExpectText("Could not unlock the note")
	b.ExpectText("cannot be decrypted")
	b.ExpectText("This note is locked")
	if sealedOf(t, grocery) != locked {
		t.Error("the encrypted note changed")
	}
}

// No keyring: nothing can be locked, the controls say why.
func TestNoKeyring(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.Secrets)
	a.Click(card("Groceries"))
	a.Click(lockNote)
	a.ExpectText("This desktop has no system keyring")
	a.ExpectValue(editor, groceriesText)
	noneLocked(t)

	// The button is off now, and its tooltip says why.
	a.Expect(lockNote, telatest.Disabled(true))
	a.MouseMove(windowW-60, windowH-120)
	b := a.Bounds(lockNote)
	a.MouseMove(b.X+b.Width/2, b.Y+b.Height/2)
	a.ExpectText("Locked notes need a system keyring")

	// The card's menu has it disabled too.
	a.Click(card("Groceries"), telatest.Right())
	a.Expect(telatest.ByRole("menuitem", "Lock"), telatest.Disabled(true))
}

func TestLockFromTheCardMenu(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("All notes"))
	a.Click(card("Groceries"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Lock"))
	a.ExpectRole("button", "Locked note, locked")
	a.ExpectCount(card("Groceries"), 0)
	noPlaintext(t, "Coffee")

	// Unlock from the same menu; "Remove lock" takes the lock off.
	a.Click(lockedCard, telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Unlock"))
	a.ExpectRole("button", "Groceries, locked")
	a.Click(card("Groceries, locked"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Remove lock"))
	a.ExpectRole("button", "Groceries")
	if n, ok := find(storedNotes(t), grocery); !ok || !strings.Contains(n.Text, "Coffee beans") || n.Sealed != "" || n.Locked {
		t.Errorf("a note without its lock is not stored in the clear again: %+v", n)
	}
}

// Deleting a locked note and undoing it brings back the encrypted note.
func TestDeleteLockedNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.Click(unlockBtn)
	// The keyring is read off the UI thread: wait for the text, or Delete
	// (there in both states) could delete the note still locked, whose
	// toast says "Deleted Locked note" (seen on a loaded CI runner).
	a.ExpectValue(editor, groceriesText)
	a.Click(deleteBtn)
	a.ExpectText("Deleted Groceries")
	a.Click(telatest.ByRole("button", "Undo"))
	a.ExpectRole("button", "Locked note, locked")
	noPlaintext(t, "Coffee", "Groceries")
}

func TestLockedNoteKeepsItsPlaceAndOrder(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.Click(railItem("Home"))
	a.ExpectCount(lockedCard, 1) // its folder is still a folder
	a.Click(railItem("All notes"))
	a.Click(card("Welcome to Notes"))
	a.Click(lockedCard)
	a.ExpectText("This note is locked")
}

// Two notes unlocked at once share the key; locking one keeps it for the
// other, and the last one to lock lets it go.
func TestTwoUnlockedNotes(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"))
	a.Click(lockNote)
	a.ExpectText("This note is locked")
	a.Click(card("Weekend in the mountains"))
	a.Click(lockNote)
	a.ExpectCount(telatest.ByRole("button", "Locked note, locked"), 2)

	a.Click(unlockBtn) // the open one: the weekend
	a.ExpectValue(titleBox, "Weekend in the mountains")
	a.Click(telatest.ByRole("button", "Locked note, locked"))
	a.Click(unlockBtn)
	a.ExpectValue(titleBox, "Groceries")

	// Both are open: an edit of one seals with the key that opened the other.
	a.Click(editor)
	a.Type("!")
	a.Click(card("Weekend in the mountains, locked"))
	a.ExpectValue(editor, "Leave on Friday after lunch, back on Sunday night. A cabin with a stove, a thermos, and no signal.\n\n#travel #ideas")
	a.Click(lockNow)
	a.Click(card("Groceries, locked")) // a locked note is never "unsaved": it is never saved to a file
	a.ExpectValue(editor, groceriesText+"!")
	a.Click(lockNow)
	noPlaintext(t, "Coffee", "cabin")
	a.ExpectCount(telatest.ByRole("button", "Locked note, locked"), 2)
}
