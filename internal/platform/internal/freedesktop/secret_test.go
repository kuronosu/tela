//go:build linux

package freedesktop

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

func TestDHPrime(t *testing.T) {
	if dhPrime == nil || dhPrime.BitLen() != 1024 || !dhPrime.ProbablyPrime(32) {
		t.Fatal("dhPrime is not a 1024-bit prime")
	}
	q := new(big.Int).Rsh(dhPrime, 1) // a safe prime: (p-1)/2 is prime too
	if !q.ProbablyPrime(32) {
		t.Fatal("dhPrime is not a safe prime")
	}
}

func TestSecretCrypto(t *testing.T) {
	a, err := newDHKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newDHKey(rand.Reader)
	ka, err := a.aesKey(b.pub)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := b.aesKey(a.pub)
	if !bytes.Equal(ka, kb) || len(ka) != 16 {
		t.Fatalf("the two sides derived %x and %x", ka, kb)
	}
	for _, bad := range [][]byte{nil, {1}, dhPrime.Bytes(), new(big.Int).Sub(dhPrime, big.NewInt(1)).Bytes()} {
		if _, err := a.aesKey(bad); err == nil {
			t.Errorf("aesKey(%x) accepted an invalid public key", bad)
		}
	}
	for n := 0; n <= 33; n++ {
		plain := bytes.Repeat([]byte{byte(n)}, n)
		iv, ct, err := sealSecret(ka, plain, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if len(ct)%16 != 0 || len(ct) <= n || len(iv) != 16 {
			t.Errorf("%d bytes sealed into %d with a %d-byte IV", n, len(ct), len(iv))
		}
		got, err := openSecret(kb, iv, ct)
		if err != nil || !bytes.Equal(got, plain) {
			t.Errorf("round trip of %d bytes = %x, %v", n, got, err)
		}
	}
	iv, ct, _ := sealSecret(ka, []byte("hello"), rand.Reader)
	iv2, ct2, _ := sealSecret(ka, []byte("hello"), rand.Reader)
	if bytes.Equal(iv, iv2) || bytes.Equal(ct, ct2) {
		t.Errorf("the IV is not fresh per secret")
	}
	wrong := bytes.Clone(kb)
	wrong[0] ^= 1
	if got, err := openSecret(wrong, iv, ct); err == nil && bytes.Equal(got, []byte("hello")) {
		t.Errorf("a wrong key decrypted the secret")
	}
	if _, err := openSecret(kb, iv[:8], ct); err == nil {
		t.Errorf("a short IV was accepted")
	}
	if _, err := openSecret(kb, iv, ct[:15]); err == nil {
		t.Errorf("a partial block was accepted")
	}
}

// --- fake Secret Service ---

type fakeItem struct {
	path   dbus.ObjectPath
	coll   dbus.ObjectPath
	label  string
	attrs  map[string]string
	value  []byte
	ctype  string
	locked bool
}

// promptAnswer is how the fake's prompts end.
type promptAnswer int

const (
	promptComplete promptAnswer = iota
	promptDismiss
	promptNever // the user never answers
)

type fakeSecrets struct {
	t    *testing.T
	conn *dbus.Conn

	mu          sync.Mutex
	refuseDH    bool
	algorithms  []string
	sessions    map[dbus.ObjectPath][]byte // AES key; nil for plain
	colls       map[dbus.ObjectPath]bool   // collection -> locked
	alias       dbus.ObjectPath            // the default alias; "/" for none
	items       []*fakeItem
	n           int
	answer      promptAnswer
	prompts     int // prompts shown
	dismissed   int // prompts dismissed by the client
	createdColl string
}

const fakeLogin = dbus.ObjectPath("/org/freedesktop/secrets/collection/login")

func startSecrets(t *testing.T, addr string) *fakeSecrets {
	t.Helper()
	f := &fakeSecrets{
		t:        t,
		conn:     connect(t, addr),
		sessions: map[dbus.ObjectPath][]byte{},
		colls:    map[dbus.ObjectPath]bool{fakeLogin: false},
		alias:    fakeLogin,
	}
	if err := f.conn.ExportMethodTable(map[string]any{
		"OpenSession": func(algo string, input dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.algorithms = append(f.algorithms, algo)
			f.n++
			path := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/secrets/session/s%d", f.n))
			switch {
			case algo == secretAlgorithmPlain:
				f.sessions[path] = nil
				return dbus.MakeVariant(""), path, nil
			case algo == secretAlgorithmDH && !f.refuseDH:
				peer, _ := input.Value().([]byte)
				k, _ := newDHKey(rand.Reader)
				aes, err := k.aesKey(peer)
				if err != nil {
					return dbus.Variant{}, "/", dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{err.Error()})
				}
				f.sessions[path] = aes
				return dbus.MakeVariant(k.pub), path, nil
			}
			return dbus.Variant{}, "/", dbus.NewError("org.freedesktop.DBus.Error.NotSupported", []any{"no " + algo})
		},
		"ReadAlias": func(name string) (dbus.ObjectPath, *dbus.Error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if name != "default" {
				return "/", nil
			}
			return f.alias, nil
		},
		"CreateCollection": func(props map[string]dbus.Variant, alias string) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
			label, _ := props[ifaceSecretColl+".Label"].Value().(string)
			path := dbus.ObjectPath("/org/freedesktop/secrets/collection/created")
			// Creating a keyring asks for its password.
			return "/", f.newPrompt(func() dbus.Variant {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.colls[path] = false
				f.createdColl = label + " as " + alias
				if alias == "default" {
					f.alias = path
				}
				f.exportCollection(path)
				return dbus.MakeVariant(path)
			}), nil
		},
		"SearchItems": func(attrs map[string]string) ([]dbus.ObjectPath, []dbus.ObjectPath, *dbus.Error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			unlocked, locked := []dbus.ObjectPath{}, []dbus.ObjectPath{}
			for _, it := range f.items {
				if matches(it.attrs, attrs) {
					if it.locked {
						locked = append(locked, it.path)
					} else {
						unlocked = append(unlocked, it.path)
					}
				}
			}
			return unlocked, locked, nil
		},
		"Unlock": func(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
			f.mu.Lock()
			anyLocked := false
			for _, o := range objects {
				if f.colls[o] || f.item(o) != nil && f.item(o).locked {
					anyLocked = true
				}
			}
			f.mu.Unlock()
			if !anyLocked {
				return objects, "/", nil
			}
			return []dbus.ObjectPath{}, f.newPrompt(func() dbus.Variant {
				f.mu.Lock()
				defer f.mu.Unlock()
				for _, o := range objects {
					if _, ok := f.colls[o]; ok {
						f.colls[o] = false
					}
					if it := f.item(o); it != nil {
						it.locked = false
					}
				}
				return dbus.MakeVariant(objects)
			}), nil
		},
		"GetSecrets": func(items []dbus.ObjectPath, session dbus.ObjectPath) (map[dbus.ObjectPath]secretValue, *dbus.Error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			key, ok := f.sessions[session]
			if !ok {
				return nil, dbus.NewError("org.freedesktop.Secret.Error.NoSession", []any{"no session"})
			}
			out := map[dbus.ObjectPath]secretValue{}
			for _, p := range items {
				it := f.item(p)
				if it == nil || it.locked {
					continue
				}
				v := secretValue{Session: session, Parameters: []byte{}, Value: it.value, ContentType: it.ctype}
				if key != nil {
					v.Parameters, v.Value, _ = sealSecret(key, it.value, rand.Reader)
				}
				out[p] = v
			}
			return out, nil
		},
	}, secretsPath, ifaceSecretService); err != nil {
		t.Fatal(err)
	}
	f.exportCollection(fakeLogin)
	own(t, f.conn, secretsName)
	return f
}

func matches(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

// item returns the item at path; f.mu must be held.
func (f *fakeSecrets) item(path dbus.ObjectPath) *fakeItem {
	for _, it := range f.items {
		if it.path == path {
			return it
		}
	}
	return nil
}

// exportCollection exports CreateItem on a collection.
func (f *fakeSecrets) exportCollection(coll dbus.ObjectPath) {
	isLocked := dbus.NewError("org.freedesktop.Secret.Error.IsLocked", []any{"locked"})
	err := f.conn.ExportMethodTable(map[string]any{
		"CreateItem": func(props map[string]dbus.Variant, secret secretValue, replace bool) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.colls[coll] {
				return "/", "/", isLocked
			}
			key, ok := f.sessions[secret.Session]
			if !ok {
				return "/", "/", dbus.NewError("org.freedesktop.Secret.Error.NoSession", []any{"no session"})
			}
			value := secret.Value
			if key != nil {
				var err error
				if value, err = openSecret(key, secret.Parameters, secret.Value); err != nil {
					return "/", "/", dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{err.Error()})
				}
			}
			label, _ := props[ifaceSecretItem+".Label"].Value().(string)
			attrs, _ := props[ifaceSecretItem+".Attributes"].Value().(map[string]string)
			if replace {
				for _, it := range f.items {
					if it.coll == coll && maps(it.attrs, attrs) {
						it.label, it.value, it.ctype = label, value, secret.ContentType
						return it.path, "/", nil
					}
				}
			}
			f.n++
			it := &fakeItem{path: dbus.ObjectPath(fmt.Sprintf("%s/%d", coll, f.n)), coll: coll, label: label, attrs: attrs, value: value, ctype: secret.ContentType}
			f.items = append(f.items, it)
			f.exportItem(it)
			return it.path, "/", nil
		},
	}, coll, ifaceSecretColl)
	if err != nil {
		f.t.Fatal(err)
	}
}

func maps(a, b map[string]string) bool { return len(a) == len(b) && matches(a, b) }

// exportItem exports Delete on an item; f.mu is held.
func (f *fakeSecrets) exportItem(it *fakeItem) {
	isLocked := dbus.NewError("org.freedesktop.Secret.Error.IsLocked", []any{"locked"})
	_ = f.conn.ExportMethodTable(map[string]any{
		"Delete": func() (dbus.ObjectPath, *dbus.Error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if it.locked {
				return "/", isLocked
			}
			for i, x := range f.items {
				if x == it {
					f.items = append(f.items[:i], f.items[i+1:]...)
				}
			}
			return "/", nil
		},
	}, it.path, ifaceSecretItem)
}

// newPrompt exports a prompt object whose completion runs done.
func (f *fakeSecrets) newPrompt(done func() dbus.Variant) dbus.ObjectPath {
	f.mu.Lock()
	f.n++
	path := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/secrets/prompt/p%d", f.n))
	f.mu.Unlock()
	completed := ifaceSecretPrompt + ".Completed"
	_ = f.conn.ExportMethodTable(map[string]any{
		"Prompt": func(window string) *dbus.Error {
			f.mu.Lock()
			f.prompts++
			answer := f.answer
			f.mu.Unlock()
			go func() {
				time.Sleep(10 * time.Millisecond) // the user types
				switch answer {
				case promptComplete:
					f.conn.Emit(path, completed, false, done())
				case promptDismiss:
					f.conn.Emit(path, completed, true, dbus.MakeVariant(""))
				}
			}()
			return nil
		},
		"Dismiss": func() *dbus.Error {
			f.mu.Lock()
			f.dismissed++
			f.mu.Unlock()
			f.conn.Emit(path, completed, true, dbus.MakeVariant(""))
			return nil
		},
	}, path, ifaceSecretPrompt)
	return path
}

func (f *fakeSecrets) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeSecrets) stored(appID, name string) *fakeItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.items {
		if matches(it.attrs, secretAttrs(appID, name)) {
			c := *it
			return &c
		}
	}
	return nil
}

func TestSecretsNoService(t *testing.T) {
	h := newHarness(t, "unix:path=/nonexistent/tela-test-bus")
	if err := h.d.SetSecret("com.example.App", "key", []byte("x")); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetSecret without a bus = %v, want ErrUnsupported", err)
	}
	h = newHarness(t, privateBus(t))
	if _, err := h.d.Secret("com.example.App", "key"); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Secret without a service = %v, want ErrUnsupported", err)
	}
	if err := h.d.DeleteSecret("com.example.App", "key"); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("DeleteSecret without a service = %v, want ErrUnsupported", err)
	}
	// The arguments are checked first.
	if err := h.d.SetSecret("com.example.App", "key", make([]byte, platform.MaxSecretSize+1)); !errors.Is(err, platform.ErrSecretTooLarge) {
		t.Errorf("SetSecret of a large secret = %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Errorf("an empty name did not panic")
		}
	}()
	_, _ = h.d.Secret("com.example.App", "")
}

func TestSecretsRoundTrip(t *testing.T) {
	addr := privateBus(t)
	f := startSecrets(t, addr)
	h := newHarness(t, addr)
	const app = "com.example.Notes"
	if _, err := h.d.Secret(app, "key"); !errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("Secret of a missing name = %v, want ErrSecretNotFound", err)
	}
	if err := h.d.DeleteSecret(app, "key"); err != nil {
		t.Errorf("DeleteSecret of a missing name = %v", err)
	}
	secret := make([]byte, platform.MaxSecretSize)
	_, _ = rand.Read(secret)
	if err := h.d.SetSecret(app, "key", secret); err != nil {
		t.Fatal(err)
	}
	it := f.stored(app, "key")
	if it == nil || !bytes.Equal(it.value, secret) || it.coll != fakeLogin ||
		it.label != "key (com.example.Notes)" || it.ctype != secretContentType ||
		!maps(it.attrs, map[string]string{"xdg:schema": "dev.telaui.Secret", "app-id": app, "name": "key"}) {
		t.Fatalf("stored item = %+v", it)
	}
	got, err := h.d.Secret(app, "key")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("Secret = %d bytes, %v", len(got), err)
	}
	// Replacing keeps one item; another app's secret of the same name is apart.
	if err := h.d.SetSecret(app, "key", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if err := h.d.SetSecret("org.other.App", "key", []byte("other")); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.d.Secret(app, "key"); string(got) != "v2" {
		t.Errorf("Secret after replacing = %q", got)
	}
	f.mu.Lock()
	n := len(f.items)
	f.mu.Unlock()
	if n != 2 {
		t.Errorf("%d items stored, want 2", n)
	}
	// An empty secret is a secret.
	if err := h.d.SetSecret(app, "empty", nil); err != nil {
		t.Fatal(err)
	}
	if got, err := h.d.Secret(app, "empty"); err != nil || len(got) != 0 {
		t.Errorf("empty secret = %q, %v", got, err)
	}
	if err := h.d.DeleteSecret(app, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.d.Secret(app, "key"); !errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("Secret after DeleteSecret = %v", err)
	}
	if got, _ := h.d.Secret("org.other.App", "key"); string(got) != "other" {
		t.Errorf("DeleteSecret removed another app's secret")
	}
	f.mu.Lock()
	algos := f.algorithms
	f.mu.Unlock()
	for _, a := range algos {
		if a != secretAlgorithmDH {
			t.Errorf("session algorithms = %q, want only the encrypted one", algos)
			break
		}
	}
	// After Close the calls are unsupported.
	h.d.Close()
	if _, err := h.d.Secret(app, "empty"); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Secret after Close = %v", err)
	}
}

func TestSecretsPlainFallback(t *testing.T) {
	addr := privateBus(t)
	f := startSecrets(t, addr)
	f.set(func() { f.refuseDH = true })
	h := newHarness(t, addr)
	if err := h.d.SetSecret("com.example.Notes", "key", []byte("plain")); err != nil {
		t.Fatal(err)
	}
	if got, err := h.d.Secret("com.example.Notes", "key"); err != nil || string(got) != "plain" {
		t.Errorf("Secret over a plain session = %q, %v", got, err)
	}
	f.mu.Lock()
	algos := f.algorithms
	f.mu.Unlock()
	if len(algos) < 2 || algos[0] != secretAlgorithmDH || algos[1] != secretAlgorithmPlain {
		t.Errorf("algorithms = %q, want the encrypted one first", algos)
	}
}

func TestSecretsPrompts(t *testing.T) {
	addr := privateBus(t)
	f := startSecrets(t, addr)
	h := newHarness(t, addr)
	const app = "com.example.Notes"

	// A locked collection is unlocked through a prompt.
	f.set(func() { f.colls[fakeLogin] = true })
	if err := h.d.SetSecret(app, "key", []byte("v")); err != nil {
		t.Fatalf("SetSecret into a locked collection = %v", err)
	}
	f.mu.Lock()
	shown, locked := f.prompts, f.colls[fakeLogin]
	f.mu.Unlock()
	if shown != 1 || locked {
		t.Errorf("prompts = %d, collection locked = %v", shown, locked)
	}

	// A locked item, and a dismissed prompt.
	f.set(func() {
		f.items[0].locked = true
		f.answer = promptDismiss
	})
	if _, err := h.d.Secret(app, "key"); !errors.Is(err, errSecretDismissed) {
		t.Errorf("Secret with a dismissed prompt = %v", err)
	}
	if err := h.d.DeleteSecret(app, "key"); !errors.Is(err, errSecretDismissed) {
		t.Errorf("DeleteSecret with a dismissed prompt = %v", err)
	}
	if f.stored(app, "key") == nil {
		t.Errorf("a dismissed DeleteSecret deleted the secret")
	}
	f.set(func() { f.answer = promptComplete })
	if got, err := h.d.Secret(app, "key"); err != nil || string(got) != "v" {
		t.Errorf("Secret of an unlocked item = %q, %v", got, err)
	}
	f.set(func() { f.items[0].locked = true })
	if err := h.d.DeleteSecret(app, "key"); err != nil || f.stored(app, "key") != nil {
		t.Errorf("DeleteSecret of a locked item = %v", err)
	}

	// No default collection: one is created with the alias.
	f.set(func() { f.alias = "/" })
	if err := h.d.SetSecret(app, "key", []byte("new")); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	created := f.createdColl
	f.mu.Unlock()
	if created != defaultCollLabel+" as default" {
		t.Errorf("created collection = %q", created)
	}
	if it := f.stored(app, "key"); it == nil || it.coll != "/org/freedesktop/secrets/collection/created" {
		t.Errorf("the secret is not in the new default collection: %+v", it)
	}

	// Closing the Desktop ends a prompt the user never answers.
	var before int
	f.set(func() {
		f.answer = promptNever
		f.items[0].locked = true
		before = f.prompts
	})
	done := make(chan error, 1)
	go func() {
		_, err := h.d.Secret(app, "key")
		done <- err
	}()
	eventually(t, "the prompt", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.prompts == before+1
	})
	h.d.Close()
	select {
	case err := <-done:
		if !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Secret ended by Close = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not end the wait for a prompt")
	}
	eventually(t, "the dismissal", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.dismissed == 1
	})
}

func TestSecretsConcurrent(t *testing.T) {
	addr := privateBus(t)
	f := startSecrets(t, addr)
	h := newHarness(t, addr)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("k%d", i)
			if err := h.d.SetSecret("com.example.Notes", name, []byte(name)); err != nil {
				errs <- err
				return
			}
			if got, err := h.d.Secret("com.example.Notes", name); err != nil || string(got) != name {
				errs <- fmt.Errorf("Secret(%s) = %q, %v", name, got, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if f.stored("com.example.Notes", "k7") == nil {
		t.Errorf("k7 not stored")
	}
}

// TestSecretsGnomeKeyring runs the calls against a real gnome-keyring-daemon
// on a private bus, with a throwaway home: it proves that the encrypted
// session interoperates (the daemon decrypts what SetSecret sent, and
// Secret decrypts what the daemon sends). Skipped without
// gnome-keyring-daemon or dbus-daemon, and in -short mode.
func TestSecretsGnomeKeyring(t *testing.T) {
	if testing.Short() {
		t.Skip("starts gnome-keyring-daemon")
	}
	exe, err := exec.LookPath("gnome-keyring-daemon")
	if err != nil {
		t.Skip("gnome-keyring-daemon is not installed")
	}
	addr := privateBus(t)
	home := t.TempDir()
	// A short runtime directory: the daemon's control socket lives there.
	rt, err := os.MkdirTemp("", "tela-gk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(rt) })
	cmd := exec.Command(exe, "--foreground", "--unlock", "--components=secrets")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_DATA_HOME=" + home + "/data",
		"XDG_CONFIG_HOME=" + home + "/config",
		"XDG_CACHE_HOME=" + home + "/cache",
		"XDG_RUNTIME_DIR=" + rt,
		"DBUS_SESSION_BUS_ADDRESS=" + addr,
	}
	cmd.Stdin = strings.NewReader("test password")
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stderr, &stderr
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start gnome-keyring-daemon: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Logf("gnome-keyring-daemon:\n%s", stderr.String())
		}
	})
	probe := connect(t, addr)
	eventually(t, "gnome-keyring-daemon on the bus", func() bool {
		var owned bool
		err := probe.BusObject().Call(busIface+".NameHasOwner", 0, secretsName).Store(&owned)
		return err == nil && owned
	})

	h := newHarness(t, addr)
	const app = "dev.telaui.Test"
	secret := []byte("correct horse battery staple, with a few more bytes than one block")
	if err := h.d.SetSecret(app, "key", secret); err != nil {
		t.Fatalf("SetSecret = %v", err)
	}
	got, err := h.d.Secret(app, "key")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("Secret = %q, %v", got, err)
	}
	// Read it back over a plain session on another connection: the daemon
	// holds the clear value, so it decrypted the encrypted session's
	// secret with the same key.
	var out dbus.Variant
	var session dbus.ObjectPath
	svc := probe.Object(secretsName, secretsPath)
	if err := svc.Call(ifaceSecretService+".OpenSession", 0, secretAlgorithmPlain, dbus.MakeVariant("")).Store(&out, &session); err != nil {
		t.Fatal(err)
	}
	var unlocked, locked []dbus.ObjectPath
	if err := svc.Call(ifaceSecretService+".SearchItems", 0, secretAttrs(app, "key")).Store(&unlocked, &locked); err != nil || len(unlocked) != 1 {
		t.Fatalf("SearchItems = %v, %v, %v", unlocked, locked, err)
	}
	var secrets map[dbus.ObjectPath]secretValue
	if err := svc.Call(ifaceSecretService+".GetSecrets", 0, unlocked, session).Store(&secrets); err != nil {
		t.Fatal(err)
	}
	if v := secrets[unlocked[0]]; !bytes.Equal(v.Value, secret) {
		t.Errorf("the daemon holds %q", v.Value)
	}
	var label dbus.Variant
	if err := probe.Object(secretsName, unlocked[0]).Call("org.freedesktop.DBus.Properties.Get", 0, ifaceSecretItem, "Label").Store(&label); err != nil || label.Value() != "key ("+app+")" {
		t.Errorf("label = %v, %v", label, err)
	}

	if err := h.d.SetSecret(app, "key", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.d.Secret(app, "key"); string(got) != "v2" {
		t.Errorf("Secret after replacing = %q", got)
	}
	if err := h.d.DeleteSecret(app, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.d.Secret(app, "key"); !errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("Secret after DeleteSecret = %v", err)
	}

	// A locked keyring needs a prompt, and this bus has no prompter
	// (gcr-prompter): the daemon ends the prompt as dismissed.
	if err := h.d.SetSecret(app, "locked", []byte("x")); err != nil {
		t.Fatal(err)
	}
	var coll dbus.ObjectPath
	if err := svc.Call(ifaceSecretService+".ReadAlias", 0, "default").Store(&coll); err != nil {
		t.Fatal(err)
	}
	var lockedObjs []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := svc.Call(ifaceSecretService+".Lock", 0, []dbus.ObjectPath{coll}).Store(&lockedObjs, &prompt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := h.d.Secret(app, "locked")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errSecretDismissed) {
			t.Errorf("Secret of a locked item without a prompter = %v, want the dismissed prompt", err)
		}
	case <-time.After(15 * time.Second):
		t.Logf("the daemon's prompt did not end; closing the Desktop")
		h.d.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		select {
		case err := <-done:
			if !errors.Is(err, platform.ErrUnsupported) {
				t.Errorf("Secret ended by Close = %v", err)
			}
		case <-ctx.Done():
			t.Fatal("Close did not end the wait for the daemon's prompt")
		}
	}
}
