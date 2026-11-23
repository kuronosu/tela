//go:build windows

package remote

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

// TestReadPollKeepsDeadlines: the receive restarted every eofPoll (the
// end of stream Windows' AF_UNIX can miss) is invisible to the caller:
// a Read without a deadline outlasts several polls and returns the data
// that arrives, a deadline beyond the polls still ends the Read with
// os.ErrDeadlineExceeded at about its time, and a deadline set while a
// Read waits ends it.
func TestReadPollKeepsDeadlines(t *testing.T) {
	a, b := connPair(t)
	go func() {
		time.Sleep(3 * eofPoll)
		b.write([]byte("late"), nil)
	}()
	buf := make([]byte, 16)
	start := time.Now()
	n, err := a.Read(buf)
	if err != nil || string(buf[:n]) != "late" {
		t.Fatalf("Read = %q, %v; want the data written after %v", buf[:n], err, 3*eofPoll)
	}
	if d := time.Since(start); d < 2*eofPoll {
		t.Fatalf("Read returned after %v, before the data was written", d)
	}

	start = time.Now()
	a.SetReadDeadline(start.Add(3 * eofPoll))
	if _, err := a.Read(buf); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read with a deadline = %v, want os.ErrDeadlineExceeded", err)
	}
	if d := time.Since(start); d < 3*eofPoll-10*time.Millisecond {
		t.Fatalf("Read with a deadline of %v ended after %v", 3*eofPoll, d)
	}

	a.SetReadDeadline(time.Time{})
	go func() {
		time.Sleep(eofPoll / 2)
		a.SetReadDeadline(time.Now())
	}()
	if _, err := a.Read(buf); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read interrupted by a deadline = %v, want os.ErrDeadlineExceeded", err)
	}

	// The end of the stream is still EOF.
	a.SetReadDeadline(time.Time{})
	b.Close()
	if _, err := a.Read(buf); err != io.EOF {
		t.Fatalf("Read after the peer's close = %v, want EOF", err)
	}
}
