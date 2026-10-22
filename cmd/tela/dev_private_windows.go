//go:build windows

package main

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// checkPrivateDir reports why dir is not a directory this user can trust
// with the builds (newBuildDirIn): a reparse point (a junction or symbolic
// link to elsewhere), or an owner other than this process's user or the
// default owner of its token (an elevated administrator's new files are
// owned by the Administrators group).
func checkPrivateDir(dir string) error {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%s is a reparse point", dir)
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fmt.Errorf("%s is not a directory", dir)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil {
		return fmt.Errorf("%s has no owner", dir)
	}
	tok := windows.GetCurrentProcessToken()
	user, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	if owner.Equals(user.User.Sid) {
		return nil
	}
	if def, err := tokenOwner(tok); err == nil && owner.Equals(def) {
		return nil
	}
	return fmt.Errorf("%s is owned by %s, not by this user", dir, owner)
}

// tokenOwner returns the default owner of the objects t creates
// (TokenOwner).
func tokenOwner(t windows.Token) (*windows.SID, error) {
	n := uint32(64)
	for range 4 {
		b := make([]byte, n)
		err := windows.GetTokenInformation(t, windows.TokenOwner, &b[0], n, &n)
		if err == nil {
			// TOKEN_OWNER: a pointer to a SID stored in the same buffer.
			sid := (*struct{ Owner *windows.SID })(unsafe.Pointer(&b[0])).Owner
			return sid.Copy()
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, err
		}
	}
	return nil, errors.New("GetTokenInformation(TokenOwner): buffer keeps growing")
}
