package win32

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure part of the secrets (ADR-209): how a secret is named in
// Credential Manager, and what its errors mean. secret_windows.go does the
// OS calls.

// Credential Manager constants.
const (
	credTypeGeneric         = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine = 2 // CRED_PERSIST_LOCAL_MACHINE

	errorNotFound           syscall.Errno = 1168 // ERROR_NOT_FOUND
	errorNoSuchLogonSession syscall.Errno = 1312 // ERROR_NO_SUCH_LOGON_SESSION
)

// credential is CREDENTIALW. The blob and the strings it points to are
// Go memory for the duration of CredWriteW, and Credential Manager memory
// (freed with CredFree) after CredReadW.
type credential struct {
	flags              uint32
	typ                uint32
	targetName         *uint16
	comment            *uint16
	lastWritten        [2]uint32 // FILETIME
	credentialBlobSize uint32
	credentialBlob     *byte
	persist            uint32
	attributeCount     uint32
	attributes         uintptr
	targetAlias        *uint16
	userName           *uint16
}

// credentialTarget is the TargetName of the credential that holds a
// secret: "<appID>/<name>". The app ID has no '/' (platform.CheckAppID),
// so the target tells apps apart.
func credentialTarget(appID, name string) string { return appID + "/" + name }

// secretError turns the error of a Credential Manager call into the
// platform's: a missing credential is ErrSecretNotFound, and a session
// without Credential Manager (a service account without a logon session)
// is ErrUnsupported; the rest is described.
func secretError(what string, err error) error {
	var errno syscall.Errno
	switch {
	case errors.As(err, &errno) && errno == errorNotFound:
		return platform.ErrSecretNotFound
	case errors.As(err, &errno) && errno == errorNoSuchLogonSession:
		return fmt.Errorf("win32: %s: %w (no logon session for Credential Manager)", what, platform.ErrUnsupported)
	}
	return fmt.Errorf("win32: %s: %w", what, err)
}
