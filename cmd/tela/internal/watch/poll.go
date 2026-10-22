package watch

import (
	"io/fs"
	"time"
)

// poller is the fallback: it walks the trees every PollInterval and
// compares each file's size, modification time and mode with the previous
// walk. A save that keeps both size and modification time (within the file
// system's timestamp resolution) is missed; editors change the time.
type poller struct {
	w     *Watcher
	stop  chan struct{}
	done  chan struct{}
	roots chan rootsReq
}

type rootsReq struct {
	roots []string
	ack   chan struct{}
}

type stamp struct {
	size int64
	mod  time.Time
	mode fs.FileMode
}

func newPoller(w *Watcher) *poller {
	p := &poller{w: w, stop: make(chan struct{}), done: make(chan struct{}), roots: make(chan rootsReq)}
	go p.loop()
	return p
}

// setRoots replaces the trees; it returns once the new baseline is taken,
// so that a change made afterwards is seen.
func (p *poller) setRoots(roots []string) error {
	ack := make(chan struct{})
	select {
	case p.roots <- rootsReq{roots, ack}:
		<-ack
	case <-p.done:
	}
	return nil
}

func (p *poller) close() error {
	close(p.stop)
	<-p.done
	return nil
}

// snapshot walks the trees.
func (p *poller) snapshot(roots []string) (map[string]stamp, map[string]bool) {
	files, dirs := map[string]stamp{}, map[string]bool{}
	for _, r := range roots {
		p.w.walk(r, func(path string) error { dirs[path] = true; return nil }, func(path string, d fs.DirEntry) {
			if fi, err := d.Info(); err == nil {
				files[path] = stamp{fi.Size(), fi.ModTime(), fi.Mode()}
			}
		})
	}
	return files, dirs
}

func (p *poller) loop() {
	defer close(p.done)
	var roots []string
	files, dirs := p.snapshot(nil)
	t := time.NewTicker(p.w.opts.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case r := <-p.roots:
			roots = r.roots
			files, dirs = p.snapshot(roots)
			close(r.ack)
			continue
		case <-t.C:
		}
		nf, nd := p.snapshot(roots)
		for path, s := range nf {
			if old, ok := files[path]; !ok || old != s {
				p.w.changed(path, false)
			}
		}
		for path := range files {
			if _, ok := nf[path]; !ok {
				p.w.changed(path, false)
			}
		}
		for path := range dirs {
			if !nd[path] {
				p.w.changed(path, true)
			}
		}
		files, dirs = nf, nd
	}
}
