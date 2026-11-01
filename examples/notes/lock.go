package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kuronosu/tela/desktop"
)

// Locked notes (phase 13, secure storage).
//
// A locked note keeps its title and its text encrypted in notes.json:
// AES-256-GCM with a random 12-byte nonce for every write, under a 32-byte
// random key that is created the first time a note is locked and lives in
// the system keyring (desktop.SetSecret, the name keyringName). The key is
// never in a file. Without a keyring (desktop.ErrUnsupported) nothing can
// be locked or unlocked.
//
// What is encrypted is content: the title, the text and Saved (the text
// the note had when it was last written to its file, which would give the
// text away). The folder, the pin, the date and the path of the file stay
// in the clear, so the library can still be listed, filtered by folder and
// sorted. The JSON of a locked note:
//
//	{"id":7,"title":"","text":"","saved":"","path":"","folder":"Home",
//	 "pinned":false,"modified":"…","locked":true,"sealed":"v1:BASE64"}
//
// where BASE64 is the nonce followed by the ciphertext and its 16-byte
// tag. The note's ID is the additional authenticated data, so a sealed
// value copied to another note does not open there.
//
// Unlocking is a session thing. A note stays unlocked, with its content in
// memory (the vault), until the user locks it again, the window is hidden
// (to the tray) or the app quits. While it is unlocked every edit is
// encrypted again and stored in the library, so notes.json never holds more
// than the ciphertext. The key stays in memory only while some note is
// unlocked. Reading the key from the keyring can ask the user to unlock it,
// so it happens off the UI thread (screen.go).

// keyringName is the name of the library key in the system keyring.
const keyringName = "library-key"

// keySize is the size of the key, in bytes: AES-256.
const keySize = 32

// sealVersion prefixes Sealed, so the format can change later.
const sealVersion = "v1:"

// content is what a locked note keeps encrypted.
type content struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Saved string `json:"saved"`
}

// errBadKey is returned when the keyring has a key that is not a key of
// this app (the wrong size).
var errBadKey = errors.New("the key in the keyring is not a key of Notes")

// errCannotOpen is returned when a sealed value does not open with the
// key: it was made with another key, or the library was changed.
var errCannotOpen = errors.New("the note cannot be decrypted with the key in the keyring")

// newKey makes a random key.
func newKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("making a key: %w", err)
	}
	return key, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, errBadKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// aad is the additional data that ties a sealed value to its note.
func aad(id int) []byte { return fmt.Appendf(nil, "notes/%d", id) }

// seal encrypts the content of the note id with a new random nonce.
func seal(key []byte, id int, c content) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(plain)+gcm.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("making a nonce: %w", err)
	}
	return sealVersion + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, aad(id))), nil
}

// unseal decrypts the content of the note id.
func unseal(key []byte, id int, sealed string) (content, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return content{}, err
	}
	raw, ok := strings.CutPrefix(sealed, sealVersion)
	if !ok {
		return content{}, fmt.Errorf("%w (unknown format)", errCannotOpen)
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < gcm.NonceSize()+gcm.Overhead() {
		return content{}, fmt.Errorf("%w (damaged)", errCannotOpen)
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], aad(id))
	if err != nil {
		return content{}, errCannotOpen
	}
	var c content
	if err := json.Unmarshal(plain, &c); err != nil {
		return content{}, fmt.Errorf("%w (damaged)", errCannotOpen)
	}
	return c, nil
}

// fetchKey reads the library key from the keyring; with create, makes and
// stores one when there is none. It blocks (the keyring may ask the user to
// unlock it), so call it off the UI thread. The errors are the keyring's:
// desktop.ErrSecretCanceled, ErrSecretNotFound, ErrUnsupported.
func fetchKey(create bool) ([]byte, error) {
	key, err := desktop.Secret(keyringName)
	switch {
	case err == nil:
		if len(key) != keySize {
			return nil, errBadKey
		}
		return key, nil
	case create && errors.Is(err, desktop.ErrSecretNotFound):
		key, err := newKey()
		if err != nil {
			return nil, err
		}
		if err := desktop.SetSecret(keyringName, key); err != nil {
			return nil, err
		}
		return key, nil
	}
	return nil, err
}

// lockedForm returns the note as the library stores it once locked: the
// content is gone and sealed is in its place.
func lockedForm(n Note, sealed string) Note {
	n.Locked, n.Unlocked, n.Sealed = true, false, sealed
	n.Title, n.Text, n.Saved = "", "", ""
	return n
}

// plainForm returns the note as the library stores it when it is not
// locked, with the content c.
func plainForm(n Note, c content) Note {
	n.Locked, n.Unlocked, n.Sealed = false, false, ""
	n.Title, n.Text, n.Saved = c.Title, c.Text, c.Saved
	return n
}

// hasLocked reports whether some note of the library is locked.
func hasLocked(ns []Note) bool {
	for _, n := range ns {
		if n.Locked {
			return true
		}
	}
	return false
}

// vault is what the session knows of the locked notes: the key and the
// content of the notes that are unlocked. It lives on the UI thread only.
type vault struct {
	key  []byte
	open map[int]content
}

// isOpen reports whether the locked note id is unlocked.
func (v *vault) isOpen(id int) bool { _, ok := v.open[id]; return ok }

// view returns n as the screen shows it: a locked note that is unlocked has
// its content back (and Unlocked set); anything else is returned as stored.
func (v *vault) view(n Note) Note {
	if c, ok := v.open[n.ID]; ok && n.Locked {
		n.Title, n.Text, n.Saved, n.Unlocked = c.Title, c.Text, c.Saved, true
	}
	return n
}

// views returns the library as the screen shows it.
func (v *vault) views(ns []Note) []Note {
	if len(v.open) == 0 {
		return ns
	}
	out := make([]Note, len(ns))
	for i, n := range ns {
		out[i] = v.view(n)
	}
	return out
}

// unlock keeps the content of the note id, and the key it opened with.
func (v *vault) unlock(id int, key []byte, c content) {
	if v.open == nil {
		v.open = map[int]content{}
	}
	v.forgetKey()
	v.key = slices.Clone(key) // a copy: forgetKey zeroes it
	v.open[id] = c
}

// lock forgets the content of the note id; the key goes too when it was the
// last unlocked note.
func (v *vault) lock(id int) {
	delete(v.open, id)
	if len(v.open) == 0 {
		v.forgetKey()
	}
}

// clear forgets everything: the window was hidden.
func (v *vault) clear() {
	v.open = nil
	v.forgetKey()
}

func (v *vault) forgetKey() {
	clear(v.key)
	v.key = nil
}

// change applies f to the content of the unlocked note id (f may change the
// Title, the Text and Saved of the note it gets) and returns the new sealed
// value for the library.
func (v *vault) change(id int, f func(*Note)) (string, error) {
	c, ok := v.open[id]
	if !ok {
		return "", fmt.Errorf("note %d is not unlocked", id)
	}
	n := Note{ID: id, Title: c.Title, Text: c.Text, Saved: c.Saved}
	f(&n)
	sealed, err := seal(v.key, id, content{n.Title, n.Text, n.Saved})
	if err != nil {
		return "", err
	}
	v.open[id] = content{n.Title, n.Text, n.Saved}
	return sealed, nil
}
