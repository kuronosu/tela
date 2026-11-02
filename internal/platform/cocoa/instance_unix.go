//go:build darwin || linux

package cocoa

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Single instance over a lock file and a Unix socket (ADR-224), with no
// AppKit: instance_darwin.go is the Desktop method on top. It builds on
// Linux too, where only its tests use it (Linux has the D-Bus claim).
//
//   - The first process takes an exclusive, non-blocking flock on the
//     lock file and holds it until it exits or closes the claim: the
//     kernel releases it when the process dies, so a crash never leaves
//     the app claimed. Owning the lock is what makes a process the first
//     one; the socket only carries the launches.
//   - Holding the lock, it removes the socket file a crashed first process
//     may have left and listens on the socket anew, mode 0600. Closing
//     the claim closes the listener (which removes the socket file) and
//     then the lock file.
//   - A second process finds the lock held and connects to the socket;
//     it reads the hello (the first's PID), sends its launch and waits
//     for the ack. While nobody accepts the connection (the first process
//     has the lock but does not listen yet, or has just exited), it tries
//     the lock and the socket again, until a deadline.
//   - Each connection has a deadline and each message a size limit, so a
//     stuck or hostile peer cannot hold either process. Both files must
//     belong to the user: in a shared folder (/tmp when TMPDIR is unset)
//     another user could create them first, and the claim then fails
//     instead of talking to that user's process.

// instanceTimeout bounds a claim's wait for the first process and each
// connection's exchange.
const instanceTimeout = 3 * time.Second

// instanceClaim is a held claim: the locked file and the listener.
type instanceClaim struct {
	lock    *os.File
	ln      *net.UnixListener
	sock    string
	deliver func(platform.Launch) bool
	closed  atomic.Bool
	wg      sync.WaitGroup // the accept loop
}

// errNoListener: the lock is held but nobody accepted a connection.
var errNoListener = errors.New("the first instance does not listen on its socket")

// claimInstance claims appID's single instance with the files in dir
// (instancePaths). When no other process holds it, it returns the claim
// and primary true; deliver then receives every forwarded launch, on a
// goroutine of the claim, until close, and reports whether it took it:
// only a launch taken is acknowledged, so a second process whose launch
// was refused (the first is shutting down) gets an error and runs on its
// own instead of exiting with its launch lost. When another process holds
// the claim, launch is forwarded to it and primary is false: hello is
// called on the calling goroutine with the first process's PID before the
// launch is sent (the darwin backend yields the activation there; nil
// skips it). An error means nothing was claimed or forwarded.
func claimInstance(dir, appID string, launch platform.Launch, deliver func(platform.Launch) bool, hello func(pid int)) (c *instanceClaim, primary bool, err error) {
	lockPath, sockPath, err := instancePaths(dir, appID)
	if err != nil {
		return nil, false, err
	}
	deadline := time.Now().Add(instanceTimeout)
	for {
		f, err := openLockFile(lockPath)
		if err != nil {
			return nil, false, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			c, err := listenInstance(f, sockPath, deliver)
			if err != nil {
				f.Close()
				return nil, false, err
			}
			return c, true, nil
		}
		f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, fmt.Errorf("locking %s: %w", lockPath, err)
		}
		err = forwardLaunch(sockPath, launch, hello, deadline)
		switch {
		case err == nil:
			return nil, false, nil
		case !errors.Is(err, errNoListener):
			return nil, false, err
		case !time.Now().Before(deadline):
			return nil, false, fmt.Errorf("another process holds %s, but %w", lockPath, err)
		}
		// Nobody listened: the first process is starting, or has exited
		// and freed the lock. Try both again.
		time.Sleep(25 * time.Millisecond)
	}
}

// openLockFile opens (creating) the lock file, which must belong to the
// user.
func openLockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening the lock file: %w", err)
	}
	if err := ownedByUser(f.Stat()); err != nil {
		f.Close()
		return nil, fmt.Errorf("the lock file %s: %w", path, err)
	}
	return f, nil
}

// ownedByUser checks that the file belongs to the user running the
// process.
func ownedByUser(fi fs.FileInfo, err error) error {
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("belongs to user %d, not to this user (%d)", st.Uid, os.Getuid())
	}
	return nil
}

// listenInstance listens on the socket for a process that holds the lock
// f, replacing a stale socket file.
func listenInstance(f *os.File, sockPath string, deliver func(platform.Launch) bool) (*instanceClaim, error) {
	if err := os.Remove(sockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("removing the stale socket %s: %w", sockPath, err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", sockPath, err)
	}
	// Private to the user (the folder usually is too); the listener
	// removes the file when it closes.
	if err := os.Chmod(sockPath, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("making %s private: %w", sockPath, err)
	}
	// For people looking at the folder: who holds the lock.
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	c := &instanceClaim{lock: f, ln: ln, sock: sockPath, deliver: deliver}
	c.wg.Add(1)
	go c.accept()
	return c, nil
}

// accept serves the connections until close.
func (c *instanceClaim) accept() {
	defer c.wg.Done()
	for {
		conn, err := c.ln.AcceptUnix()
		if err != nil {
			if c.closed.Load() || errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(10 * time.Millisecond) // EMFILE and the like
			continue
		}
		go c.serve(conn)
	}
}

// serve answers one second process: hello, its launch, the ack. A
// malformed launch, or one deliver does not take, gets no ack.
func (c *instanceClaim) serve(conn *net.UnixConn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(instanceTimeout))
	if _, err := conn.Write(appendFrame(nil, msgHello, encodeHello(os.Getpid()))); err != nil {
		return
	}
	kind, body, err := readFrame(conn)
	if err != nil || kind != msgLaunch {
		return
	}
	l, err := decodeLaunch(body)
	if err != nil {
		return
	}
	if c.closed.Load() || c.deliver == nil || !c.deliver(l) {
		return
	}
	_, _ = conn.Write(appendFrame(nil, msgAck, nil))
}

// close ends the claim: the listener closes (removing the socket file),
// then the lock is released. Launches still being received are dropped.
// Idempotent.
func (c *instanceClaim) close() {
	if c == nil || c.closed.Swap(true) {
		return
	}
	c.ln.Close()
	c.wg.Wait()
	c.lock.Close()
}

// forwardLaunch sends launch to the process listening on sockPath and
// waits for its ack. It connects once: errNoListener when the socket is
// missing or refuses the connection (a first process that has not
// listened yet, or has exited), which the caller retries.
func forwardLaunch(sockPath string, launch platform.Launch, hello func(pid int), deadline time.Time) error {
	// The socket must be the user's (see the top of the file).
	err := ownedByUser(os.Lstat(sockPath))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w (no socket yet)", errNoListener)
	}
	if err != nil {
		return fmt.Errorf("the socket %s: %w", sockPath, err)
	}
	conn, err := net.DialTimeout("unix", sockPath, max(time.Until(deadline), 100*time.Millisecond))
	if err != nil {
		return fmt.Errorf("%w (%v)", errNoListener, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(instanceTimeout))
	kind, body, err := readFrame(conn)
	if err != nil || kind != msgHello {
		return fmt.Errorf("the first instance did not greet: %v (message %d)", err, kind)
	}
	pid, err := decodeHello(body)
	if err != nil {
		return err
	}
	if hello != nil {
		hello(pid)
	}
	if _, err := conn.Write(appendFrame(nil, msgLaunch, encodeLaunch(launch))); err != nil {
		return fmt.Errorf("sending the launch to the first instance (PID %d): %w", pid, err)
	}
	kind, _, err = readFrame(conn)
	if err != nil || kind != msgAck {
		return fmt.Errorf("the first instance (PID %d) did not acknowledge the launch: %v (message %d)", pid, err, kind)
	}
	return nil
}
