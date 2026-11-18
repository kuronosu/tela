package pkgfmt

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// CommandSigner is -sign-cmd: it runs a command on each file to sign.
// Args is the command and its arguments; every argument "{}" is replaced
// by the file's absolute path, and without one the path is the last
// argument. The command signs the file in place, or writes a signature
// next to it (gpg --detach-sign); a non-zero exit status is an error.
type CommandSigner struct {
	Args []string
	// Out receives the command's output.
	Out io.Writer
}

func (c CommandSigner) Sign(path, what string, t Target) (bool, error) {
	if len(c.Args) == 0 {
		return false, nil
	}
	args := make([]string, 0, len(c.Args)+1)
	placed := false
	for _, a := range c.Args[1:] {
		if a == "{}" {
			a, placed = path, true
		}
		args = append(args, a)
	}
	if !placed {
		args = append(args, path)
	}
	cmd := exec.Command(c.Args[0], args...)
	cmd.Stdout, cmd.Stderr = c.Out, c.Out
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("-sign-cmd %s: %v", strings.Join(c.Args, " "), err)
	}
	return true, nil
}

// Chain runs every signer on a file; it is signed when one of them signed
// it.
type Chain []Signer

func (ch Chain) Sign(path, what string, t Target) (bool, error) {
	signed := false
	for _, s := range ch {
		ok, err := s.Sign(path, what, t)
		if err != nil {
			return signed, err
		}
		signed = signed || ok
	}
	return signed, nil
}
