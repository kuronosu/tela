package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/desktop"
)

// Download fetches the update's artifact into the app's cache folder
// (desktop.CacheDir, under update/) and checks its size and SHA-256
// against the manifest's: a file that differs is deleted
// ([ErrHashMismatch]). progress, when not nil, is called on Download's
// goroutine as bytes arrive, with the bytes done and the total.
//
// An interrupted download resumes where it stopped on the next call when
// the server supports range requests, and a file already downloaded is
// not fetched again. Cancel it with ctx.
func (u *Update) Download(ctx context.Context, progress func(done, total int64)) error {
	if len(u.Artifact.SHA256) != sha256.Size*2 || u.Artifact.Size < 0 {
		return fmt.Errorf("update: the manifest's artifact has no valid sha256 and size")
	}
	dir, err := desktop.CacheDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "update")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	final := filepath.Join(dir, strings.ToLower(u.Artifact.SHA256))
	total := u.Artifact.Size
	if verifyFile(final, total, u.Artifact.SHA256) == nil {
		u.file = final
		if progress != nil {
			progress(total, total)
		}
		return nil
	}
	part := final + ".part"
	h := sha256.New()
	var done int64
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	// Resume: hash what is there.
	if n, err := io.Copy(h, f); err == nil && n > 0 && n < total {
		done = n
	} else {
		h.Reset()
		if err := f.Truncate(0); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if done > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(done, 10)+"-")
	}
	res, err := u.cfg.client().Do(req)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	defer res.Body.Close()
	switch {
	case done > 0 && res.StatusCode == http.StatusPartialContent && strings.HasPrefix(res.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(done, 10)+"-"):
	case res.StatusCode == http.StatusOK:
		if done > 0 { // the server ignored the range: start over
			done = 0
			h.Reset()
			if err := f.Truncate(0); err != nil {
				return err
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("update: GET %s: %s", u.url, res.Status)
	}
	if progress != nil {
		progress(done, total)
	}
	w := &progressWriter{w: io.MultiWriter(f, h), done: done, total: total, progress: progress}
	_, err = io.Copy(w, io.LimitReader(res.Body, total-done+1))
	if err != nil {
		return fmt.Errorf("update: download: %w", err) // keep the part for a resume
	}
	if w.done > total {
		f.Close() // Windows deletes no open file
		os.Remove(part)
		return fmt.Errorf("%w: the file is larger than %d bytes", ErrHashMismatch, total)
	}
	if w.done < total {
		return fmt.Errorf("update: download: %w after %d of %d bytes", io.ErrUnexpectedEOF, w.done, total)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, u.Artifact.SHA256) {
		f.Close() // Windows deletes no open file
		os.Remove(part)
		return fmt.Errorf("%w: SHA-256 %s, want %s", ErrHashMismatch, got, u.Artifact.SHA256)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(part, final); err != nil {
		return err
	}
	u.file = final
	return nil
}

type progressWriter struct {
	w           io.Writer
	done, total int64
	progress    func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.progress != nil && n > 0 {
		p.progress(min(p.done, p.total), p.total)
	}
	return n, err
}

// verifyFile checks the size and SHA-256 of the file at path.
func verifyFile(path string, size int64, sum string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() != size {
		return fmt.Errorf("%w: %s has %d bytes, want %d", ErrHashMismatch, path, st.Size(), size)
	}
	var h hash.Hash = sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, sum) {
		return fmt.Errorf("%w: SHA-256 of %s is %s, want %s", ErrHashMismatch, path, got, sum)
	}
	return nil
}

// errNotDownloaded is Apply's before Download.
var errNotDownloaded = errors.New("update: Apply before Download")
