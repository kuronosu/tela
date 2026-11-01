package desktop

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Secrets (ADR-209, ADR-213): small secrets kept in the OS's secure
// storage, named by the app ID and a name.

// MaxSecretSize is the largest secret, in bytes, every OS stores (the
// limit of the Windows Credential Manager). Keep keys and tokens in
// secrets; encrypt larger data with a key kept there.
const MaxSecretSize = platform.MaxSecretSize

// ErrSecretNotFound is returned by [Secret] when no secret has the name.
// errors.Is(err, fs.ErrNotExist) also holds.
var ErrSecretNotFound = platform.ErrSecretNotFound

// ErrSecretCanceled is returned by the secret functions when the user
// canceled the OS's prompt to unlock the keyring (Linux): nothing was
// read or written. Asking again later is fine; do not ask in a loop.
var ErrSecretCanceled = platform.ErrSecretCanceled

// ErrSecretTooLarge is returned by [SetSecret] for a secret larger than
// MaxSecretSize.
var ErrSecretTooLarge = platform.ErrSecretTooLarge

// SetSecret stores secret under name in the OS's secure storage, for this
// app ([AppID]), replacing an earlier value: the Credential Manager on
// Windows, the Secret Service (GNOME Keyring, KWallet) on Linux. Use it
// for passwords, tokens and encryption keys, never in a plain file.
//
//	tela.UseGo(func(ctx context.Context) {
//	    if err := desktop.SetSecret("sync-token", token); err != nil {
//	        tela.Post(func() { setError.Set(err.Error()) })
//	    }
//	}, token)
//
// The secret functions block: the OS may show a prompt to unlock the
// keyring and wait for the user. Call them from a goroutine (tela.UseGo),
// not from a handler or an effect, which would freeze the app; they are
// safe from any goroutine, and dev mode warns when one runs on the UI
// thread. A desktop without secure storage returns an error wrapping
// [ErrUnsupported]; a canceled prompt [ErrSecretCanceled]. They need a
// running app (tela.Run or telatest). Tests preset and read secrets with
// telatest's App.SetStoredSecret and App.StoredSecret.
func SetSecret(name string, secret []byte) error {
	checkSecretName("SetSecret", name)
	svc, err := secretServices("SetSecret")
	if err != nil {
		return err
	}
	return wrap(fmt.Sprintf("SetSecret(%q)", name), svc.SetSecret(serviceID(AppID()), name, secret))
}

// Secret returns the secret stored under name for this app, or an error
// wrapping [ErrSecretNotFound]. It blocks; see [SetSecret].
//
//	key, err := desktop.Secret("notes-key")
//	switch {
//	case errors.Is(err, desktop.ErrSecretNotFound):
//	    key = newKey() // first run
//	case errors.Is(err, desktop.ErrUnsupported):
//	    disableLockedNotes()
//	}
func Secret(name string) ([]byte, error) {
	checkSecretName("Secret", name)
	svc, err := secretServices("Secret")
	if err != nil {
		return nil, err
	}
	b, err := svc.Secret(serviceID(AppID()), name)
	return b, wrap(fmt.Sprintf("Secret(%q)", name), err)
}

// DeleteSecret removes the secret stored under name for this app; a
// missing one is not an error. It blocks; see [SetSecret].
func DeleteSecret(name string) error {
	checkSecretName("DeleteSecret", name)
	svc, err := secretServices("DeleteSecret")
	if err != nil {
		return err
	}
	return wrap(fmt.Sprintf("DeleteSecret(%q)", name), svc.DeleteSecret(serviceID(AppID()), name))
}

// checkSecretName panics with the fix for a name the OS cannot take.
func checkSecretName(what, name string) {
	if name == "" || len(name) > 255 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		panic(fmt.Sprintf("desktop.%s(%q): a secret's name is non-empty text without control characters, at most 255 bytes (\"sync-token\")", what, name))
	}
}

// secretServices returns the running app's services for a secret call,
// warning in dev mode when the call runs on the UI thread.
func secretServices(what string) (platform.Desktop, error) {
	svc, d, onUI, ok := core.RunningServices()
	if !ok {
		return nil, errors.New("desktop." + what + " needs a running app (tela.Run, or telatest in tests)")
	}
	if onUI {
		d.WarnAnyGoroutine("desktop-secret-ui", func() string {
			return "desktop." + what + " runs on the UI thread: it blocks while the OS asks the user to unlock the keyring, freezing the app; call it from a goroutine (tela.UseGo) and give the result back with tela.Post or a state setter"
		})
	}
	return svc, nil
}
