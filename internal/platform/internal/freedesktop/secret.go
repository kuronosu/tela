//go:build linux

package freedesktop

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// Secrets: the Secret Service API (org.freedesktop.secrets), served by
// gnome-keyring, KeePassXC, KWallet 5.97+ and others.
//
// Unlike the other services, SetSecret, Secret and DeleteSecret block and
// are safe from any goroutine (the contract's exception). So they do not
// share the UI thread's connection: each call dials its own, opens a
// session, does its work and closes the connection, which also closes
// the session on the service's side. The calls are rare (an app reads a
// key at start and writes it once), so the round trips do not matter.
//
// A call:
//
//  1. Checks that the service is on the bus or activatable; otherwise
//     ErrUnsupported.
//  2. Opens an encrypted session, dh-ietf1024-sha256-aes128-cbc-pkcs7
//     (secretcrypto.go). Only a service that answers
//     org.freedesktop.DBus.Error.NotSupported for it gets the "plain"
//     algorithm instead, as libsecret falls back: the secret then crosses
//     the session bus in the clear, where only this user's processes (and
//     bus monitors, which need the same user) can see it. Any other error
//     fails the call.
//  3. SetSecret: the default collection (ReadAlias "default"; created as
//     "Default keyring" with that alias when missing, as libsecret does),
//     unlocked (Unlock), then Collection.CreateItem with replace, the label
//     "<name> (<app ID>)" and the attributes xdg:schema
//     "dev.telaui.Secret", app-id and name. The content type is
//     application/octet-stream.
//  4. Secret: Service.SearchItems with the attributes (every collection),
//     preferring an item of the default collection; a locked item is
//     unlocked first; then Service.GetSecrets. No item is
//     ErrSecretNotFound.
//  5. DeleteSecret: SearchItems, Unlock of the locked items, Item.Delete
//     of every match. None is not an error.
//
// Unlock, CreateCollection, CreateItem and Delete may return a prompt
// instead of a result: the call subscribes to the prompt's Completed
// signal, calls Prompt (no parent window: these calls have none), and
// waits without a time limit, since the user is typing a password. A
// dismissed prompt is an error (errSecretDismissed); closing the Desktop
// dismisses a pending prompt and ends the call with ErrUnsupported.

const (
	secretsName          = "org.freedesktop.secrets"
	secretsPath          = dbus.ObjectPath("/org/freedesktop/secrets")
	ifaceSecretService   = "org.freedesktop.Secret.Service"
	ifaceSecretColl      = "org.freedesktop.Secret.Collection"
	ifaceSecretItem      = "org.freedesktop.Secret.Item"
	ifaceSecretPrompt    = "org.freedesktop.Secret.Prompt"
	secretAlgorithmDH    = "dh-ietf1024-sha256-aes128-cbc-pkcs7"
	secretAlgorithmPlain = "plain"
	secretSchema         = "dev.telaui.Secret"
	secretContentType    = "application/octet-stream"
	defaultCollLabel     = "Default keyring"
	noPrompt             = dbus.ObjectPath("/")
)

// secretCallTimeout bounds each Secret Service method call (not the wait
// for a prompt). The first call may start the service through D-Bus
// activation.
const secretCallTimeout = 25 * time.Second

// errSecretDismissed is returned when the user dismisses the Secret
// Service's prompt (an unlock or a new keyring's password).
var errSecretDismissed = fmt.Errorf("freedesktop: the keyring prompt was dismissed: %w", platform.ErrSecretCanceled)

// secretValue is the Secret struct of the API: (oayays).
type secretValue struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// secretClient is one call's connection and session.
type secretClient struct {
	d       *Desktop
	conn    *dbus.Conn
	signals chan *dbus.Signal
	session dbus.ObjectPath
	key     []byte // the session's AES key; nil for "plain"
}

// SetSecret stores secret in the Secret Service (see secret.go).
func (d *Desktop) SetSecret(appID, name string, secret []byte) error {
	if err := platform.CheckSecret(appID, name, secret); err != nil {
		return err
	}
	c, err := d.openSecrets(true)
	if err != nil {
		return err
	}
	defer c.close()
	coll, err := c.defaultCollection()
	if err != nil {
		return err
	}
	if err := c.unlock([]dbus.ObjectPath{coll}); err != nil {
		return err
	}
	value, err := c.encode(secret)
	if err != nil {
		return err
	}
	props := map[string]dbus.Variant{
		ifaceSecretItem + ".Label":      dbus.MakeVariant(name + " (" + appID + ")"),
		ifaceSecretItem + ".Attributes": dbus.MakeVariant(secretAttrs(appID, name)),
	}
	var item, prompt dbus.ObjectPath
	if err := c.call(coll, ifaceSecretColl+".CreateItem", props, value, true).Store(&item, &prompt); err != nil {
		return secretError("CreateItem", err)
	}
	_, err = c.prompt(prompt)
	return err
}

// Secret reads a secret from the Secret Service (see secret.go).
func (d *Desktop) Secret(appID, name string) ([]byte, error) {
	platform.CheckSecretName("Secret", appID, name)
	c, err := d.openSecrets(true)
	if err != nil {
		return nil, err
	}
	defer c.close()
	unlocked, locked, err := c.search(appID, name)
	if err != nil {
		return nil, err
	}
	if len(unlocked)+len(locked) == 0 {
		return nil, platform.ErrSecretNotFound
	}
	var coll dbus.ObjectPath
	if err := c.call(secretsPath, ifaceSecretService+".ReadAlias", "default").Store(&coll); err != nil {
		return nil, secretError("ReadAlias", err)
	}
	item, isLocked := pickItem(unlocked, locked, coll)
	if isLocked {
		if err := c.unlock([]dbus.ObjectPath{item}); err != nil {
			return nil, err
		}
	}
	var secrets map[dbus.ObjectPath]secretValue
	if err := c.call(secretsPath, ifaceSecretService+".GetSecrets", []dbus.ObjectPath{item}, c.session).Store(&secrets); err != nil {
		return nil, secretError("GetSecrets", err)
	}
	v, ok := secrets[item]
	if !ok {
		return nil, fmt.Errorf("freedesktop: the Secret Service returned no secret for %s (still locked?)", item)
	}
	return c.decode(v)
}

// DeleteSecret removes a secret from the Secret Service (see secret.go).
func (d *Desktop) DeleteSecret(appID, name string) error {
	platform.CheckSecretName("DeleteSecret", appID, name)
	c, err := d.openSecrets(false)
	if err != nil {
		return err
	}
	defer c.close()
	unlocked, locked, err := c.search(appID, name)
	if err != nil {
		return err
	}
	if len(locked) > 0 {
		if err := c.unlock(locked); err != nil {
			return err
		}
	}
	for _, item := range append(unlocked, locked...) {
		var prompt dbus.ObjectPath
		if err := c.call(item, ifaceSecretItem+".Delete").Store(&prompt); err != nil {
			return secretError("Delete", err)
		}
		if _, err := c.prompt(prompt); err != nil {
			return err
		}
	}
	return nil
}

func secretAttrs(appID, name string) map[string]string {
	return map[string]string{"xdg:schema": secretSchema, "app-id": appID, "name": name}
}

// pickItem chooses the item to read among the matches: one of the default
// collection coll first (SetSecret writes there), an unlocked one first.
func pickItem(unlocked, locked []dbus.ObjectPath, coll dbus.ObjectPath) (dbus.ObjectPath, bool) {
	inColl := func(p dbus.ObjectPath) bool {
		return coll != noPrompt && strings.HasPrefix(string(p), string(coll)+"/")
	}
	for _, p := range unlocked {
		if inColl(p) {
			return p, false
		}
	}
	for _, p := range locked {
		if inColl(p) {
			return p, true
		}
	}
	if len(unlocked) > 0 {
		return unlocked[0], false
	}
	return locked[0], true
}

// openSecrets connects to the session bus for one secret call and, with
// session, opens a session. A missing bus or service is ErrUnsupported.
// Safe from any goroutine: it reads only Options, which never change.
func (d *Desktop) openSecrets(session bool) (*secretClient, error) {
	if d.isClosed() {
		return nil, errClosed
	}
	conn, err := d.dial()
	if err != nil {
		return nil, err
	}
	c := &secretClient{d: d, conn: conn, signals: make(chan *dbus.Signal, 16)}
	conn.Signal(c.signals)
	if !c.present() {
		conn.Close()
		return nil, unsupported("no Secret Service (%s) on the session bus; install and start a keyring such as gnome-keyring or KeePassXC", secretsName)
	}
	if session {
		if err := c.openSession(); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return c, nil
}

func (c *secretClient) close() { c.conn.Close() }

// call calls a method of the service with a timeout.
func (c *secretClient) call(path dbus.ObjectPath, method string, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), secretCallTimeout)
	defer cancel()
	return c.conn.Object(secretsName, path).CallWithContext(ctx, method, 0, args...)
}

// present reports whether the service is on the bus or activatable.
func (c *secretClient) present() bool {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var owned bool
	if err := c.conn.BusObject().CallWithContext(ctx, busIface+".NameHasOwner", 0, secretsName).Store(&owned); err == nil && owned {
		return true
	}
	var names []string
	if err := c.conn.BusObject().CallWithContext(ctx, busIface+".ListActivatableNames", 0).Store(&names); err != nil {
		return false
	}
	for _, n := range names {
		if n == secretsName {
			return true
		}
	}
	return false
}

// secretError wraps an error of the service; a service that went away is
// ErrUnsupported.
func secretError(op string, err error) error {
	var derr dbus.Error
	if errors.As(err, &derr) {
		switch derr.Name {
		case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner":
			return unsupported("the Secret Service went away (%v)", err)
		}
	}
	return fmt.Errorf("freedesktop: Secret Service %s: %w", op, err)
}

func isDBusError(err error, name string) bool {
	var derr dbus.Error
	return errors.As(err, &derr) && derr.Name == name
}

// openSession opens the encrypted session, or a plain one when the
// service does not support the encrypted algorithm.
func (c *secretClient) openSession() error {
	key, err := newDHKey(rand.Reader)
	if err != nil {
		return err
	}
	var out dbus.Variant
	var path dbus.ObjectPath
	err = c.call(secretsPath, ifaceSecretService+".OpenSession", secretAlgorithmDH, dbus.MakeVariant(key.pub)).Store(&out, &path)
	if err == nil {
		peer, ok := out.Value().([]byte)
		if !ok {
			return fmt.Errorf("freedesktop: the Secret Service answered OpenSession with %s, not a public key", out.Signature())
		}
		if c.key, err = key.aesKey(peer); err != nil {
			return err
		}
		c.session = path
		return nil
	}
	if !isDBusError(err, "org.freedesktop.DBus.Error.NotSupported") {
		return secretError("OpenSession", err)
	}
	if err := c.call(secretsPath, ifaceSecretService+".OpenSession", secretAlgorithmPlain, dbus.MakeVariant("")).Store(&out, &path); err != nil {
		return secretError("OpenSession", err)
	}
	c.session = path
	return nil
}

// encode makes the Secret struct of a value for the session.
func (c *secretClient) encode(value []byte) (secretValue, error) {
	s := secretValue{Session: c.session, ContentType: secretContentType}
	if c.key == nil {
		s.Parameters, s.Value = []byte{}, value
		if s.Value == nil {
			s.Value = []byte{}
		}
		return s, nil
	}
	var err error
	s.Parameters, s.Value, err = sealSecret(c.key, value, rand.Reader)
	return s, err
}

// decode returns the value of a Secret struct of the session.
func (c *secretClient) decode(s secretValue) ([]byte, error) {
	if c.key == nil {
		return s.Value, nil
	}
	return openSecret(c.key, s.Parameters, s.Value)
}

// defaultCollection returns the collection of the "default" alias,
// creating it (which may prompt for the new keyring's password) when
// there is none.
func (c *secretClient) defaultCollection() (dbus.ObjectPath, error) {
	var coll dbus.ObjectPath
	if err := c.call(secretsPath, ifaceSecretService+".ReadAlias", "default").Store(&coll); err != nil {
		return "", secretError("ReadAlias", err)
	}
	if coll != noPrompt && coll != "" {
		return coll, nil
	}
	props := map[string]dbus.Variant{ifaceSecretColl + ".Label": dbus.MakeVariant(defaultCollLabel)}
	var prompt dbus.ObjectPath
	if err := c.call(secretsPath, ifaceSecretService+".CreateCollection", props, "default").Store(&coll, &prompt); err != nil {
		return "", secretError("CreateCollection", err)
	}
	if coll != noPrompt && coll != "" {
		return coll, nil
	}
	result, err := c.prompt(prompt)
	if err != nil {
		return "", err
	}
	if p, ok := result.Value().(dbus.ObjectPath); ok && p != noPrompt {
		return p, nil
	}
	return "", errors.New("freedesktop: the Secret Service created no default collection")
}

// search returns the items with the app's attributes.
func (c *secretClient) search(appID, name string) (unlocked, locked []dbus.ObjectPath, err error) {
	if err := c.call(secretsPath, ifaceSecretService+".SearchItems", secretAttrs(appID, name)).Store(&unlocked, &locked); err != nil {
		return nil, nil, secretError("SearchItems", err)
	}
	return unlocked, locked, nil
}

// unlock unlocks objects (collections or items), waiting for the prompt
// when the service needs the user.
func (c *secretClient) unlock(objects []dbus.ObjectPath) error {
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := c.call(secretsPath, ifaceSecretService+".Unlock", objects).Store(&unlocked, &prompt); err != nil {
		return secretError("Unlock", err)
	}
	_, err := c.prompt(prompt)
	return err
}

// promptPoll is how often a wait for a prompt checks that the Desktop is
// still open.
const promptPoll = 250 * time.Millisecond

// prompt shows the prompt at path ("/" is none) and waits for its
// Completed signal, returning its result.
func (c *secretClient) prompt(path dbus.ObjectPath) (dbus.Variant, error) {
	if path == noPrompt || path == "" {
		return dbus.Variant{}, nil
	}
	// Subscribe before Prompt: Completed may follow at once.
	if err := addMatch(c.conn, dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(ifaceSecretPrompt), dbus.WithMatchMember("Completed")); err != nil {
		return dbus.Variant{}, secretError("AddMatch", err)
	}
	if err := c.call(path, ifaceSecretPrompt+".Prompt", "").Err; err != nil {
		return dbus.Variant{}, secretError("Prompt", err)
	}
	tick := time.NewTicker(promptPoll)
	defer tick.Stop()
	for {
		select {
		case s, ok := <-c.signals:
			if !ok {
				return dbus.Variant{}, unsupported("the session bus closed while a keyring prompt was open")
			}
			if s.Path != path || s.Name != ifaceSecretPrompt+".Completed" || len(s.Body) < 2 {
				continue
			}
			if dismissed, _ := s.Body[0].(bool); dismissed {
				return dbus.Variant{}, errSecretDismissed
			}
			result, _ := s.Body[1].(dbus.Variant)
			return result, nil
		case <-tick.C:
			if c.d.isClosed() {
				c.call(path, ifaceSecretPrompt+".Dismiss")
				return dbus.Variant{}, errClosed
			}
		}
	}
}
