// Copyright (c) 2009 The XGB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// Vendored from github.com/jezek/xgb v1.3.1 (Tela ADR-025). Tela's changes
// are marked "Tela patch" and listed in THIRD_PARTY.md.

package xgb

/*
auth.go contains functions to facilitate the parsing of .Xauthority files.

It is largely unmodified from the original XGB package that I forked.
*/

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// readAuthority reads the X authority file for the DISPLAY.
// If hostname == "" or hostname == "localhost",
// then use the system's hostname (as returned by os.Hostname) instead.
func readAuthority(hostname, display string) (
	name string, data []byte, err error) {

	// b is a scratch buffer to use and should be at least 256 bytes long
	// (i.e. it should be able to hold a hostname).
	b := make([]byte, 256)

	// As per /usr/include/X11/Xauth.h.
	const familyLocal = 256
	const familyWild = 65535

	if len(hostname) == 0 || hostname == "localhost" {
		hostname, err = os.Hostname()
		if err != nil {
			return "", nil, err
		}
	}

	fname := os.Getenv("XAUTHORITY")
	if len(fname) == 0 {
		home := os.Getenv("HOME")
		if len(home) == 0 {
			err = errors.New("Xauthority not found: $XAUTHORITY, $HOME not set")
			return "", nil, err
		}
		fname = home + "/.Xauthority"
	}

	r, err := os.Open(fname)
	if err != nil {
		return "", nil, err
	}
	defer r.Close()

	for {
		var family uint16
		if err := binary.Read(r, binary.BigEndian, &family); err != nil {
			return "", nil, err
		}

		addr, err := getString(r, b)
		if err != nil {
			return "", nil, err
		}

		disp, err := getString(r, b)
		if err != nil {
			return "", nil, err
		}

		name0, err := getString(r, b)
		if err != nil {
			return "", nil, err
		}

		data0, err := getBytes(r, b)
		if err != nil {
			return "", nil, err
		}

		addrmatch := (family == familyWild) ||
			(family == familyLocal && addr == hostname)
		dispmatch := (disp == "") || (disp == display)

		if addrmatch && dispmatch {
			return name0, data0, nil
		}
	}
	// Tela patch: `panic("unreachable")` removed here; go vet reports it
	// as unreachable code after the infinite loop.
}

// readAuthorityFromCookieHex decodes a hexadecimal MIT-MAGIC-COOKIE-1 string.
// It expects a 32-character hex string (16 bytes) and returns the decoded cookie data.
// Returns an error if the hex string is empty, invalid, or not exactly 16 bytes.
func readAuthorityFromCookieHex(cookieHex string) (name string, data []byte, err error) {
	if cookieHex == "" {
		return "", nil, errors.New("cookie hex string is required")
	}

	authData, err := hex.DecodeString(cookieHex)
	if err != nil {
		return "", nil, fmt.Errorf("invalid cookie hex: %v", err)
	}

	if len(authData) != 16 {
		return "", nil, errors.New("cookie must be 16 bytes (32 hex chars)")
	}

	return "MIT-MAGIC-COOKIE-1", authData, nil
}

func getBytes(r io.Reader, b []byte) ([]byte, error) {
	var n uint16
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return nil, err
	} else if n > uint16(len(b)) {
		return nil, errors.New("bytes too long for buffer")
	}

	if _, err := io.ReadFull(r, b[0:n]); err != nil {
		return nil, err
	}
	return b[0:n], nil
}

func getString(r io.Reader, b []byte) (string, error) {
	b, err := getBytes(r, b)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
