package assoc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// recently-used.xbel (ADR-298), against temporary directories only.

var xbelNow = time.Date(2026, 11, 19, 10, 20, 30, 123456000, time.UTC)

// gtkXBEL is a file as GLib 2.80 writes it, with another application's
// bookmark (and a private, grouped one), to merge with.
const gtkXBEL = `<?xml version="1.0" encoding="UTF-8"?>
<xbel version="1.0"
      xmlns:bookmark="http://www.freedesktop.org/standards/desktop-bookmarks"
      xmlns:mime="http://www.freedesktop.org/standards/shared-mime-info"
>
  <bookmark href="file:///home/ana/Pictures/cat%20(1).png" added="2026-11-18T09:00:00.000001Z" modified="2026-11-18T09:00:00.000001Z" visited="2026-11-18T09:00:00.000001Z">
    <info>
      <metadata owner="http://freedesktop.org">
        <mime:mime-type type="image/png"/>
        <bookmark:groups>
          <bookmark:group>Graphics</bookmark:group>
        </bookmark:groups>
        <bookmark:applications>
          <bookmark:application name="Image Viewer" exec="&apos;loupe %u&apos;" modified="2026-11-18T09:00:00.000001Z" count="2"/>
        </bookmark:applications>
        <bookmark:private/>
      </metadata>
    </info>
  </bookmark>
  <bookmark href="file:///home/ana/notes/list.md" added="2026-11-17T08:00:00.000000Z" modified="2026-11-17T08:00:00.000000Z" visited="2026-11-17T08:00:00.000000Z">
    <info>
      <metadata owner="http://freedesktop.org">
        <mime:mime-type type="text/markdown"/>
        <bookmark:applications>
          <bookmark:application name="org.gnome.TextEditor" exec="&apos;gnome-text-editor %u&apos;" timestamp="1795334400" count="1"/>
        </bookmark:applications>
      </metadata>
    </info>
  </bookmark>
</xbel>`

func xbelDirs(t *testing.T) LinuxDirs {
	return LinuxDirs{DataHome: filepath.Join(t.TempDir(), "data")}
}

func readXBEL(t *testing.T, dirs LinuxDirs) string {
	t.Helper()
	b, err := os.ReadFile(RecentPath(dirs))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestXBELNewFile(t *testing.T) {
	dirs := xbelDirs(t)
	app := notesApp
	app.Exec = "/opt/My Notes/notes"
	if err := LinuxAddRecent(dirs, app, "/home/ana/notes/a b#1.md", "text/markdown", xbelNow); err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<xbel version="1.0" xmlns:bookmark="http://www.freedesktop.org/standards/desktop-bookmarks" xmlns:mime="http://www.freedesktop.org/standards/shared-mime-info">
  <bookmark href="file:///home/ana/notes/a%20b%231.md" added="2026-11-19T10:20:30.123456Z" modified="2026-11-19T10:20:30.123456Z" visited="2026-11-19T10:20:30.123456Z">
    <info>
      <metadata owner="http://freedesktop.org">
        <mime:mime-type type="text/markdown"/>
        <bookmark:applications>
          <bookmark:application name="com.example.Notes" exec="&apos;&apos;\&apos;&apos;/opt/My Notes/notes&apos;\&apos;&apos; %u&apos;" modified="2026-11-19T10:20:30.123456Z" count="1"/>
        </bookmark:applications>
      </metadata>
    </info>
  </bookmark>
</xbel>
`
	if got := readXBEL(t, dirs); got != want {
		t.Errorf("new file:\n%s\nwant:\n%s", got, want)
	}
	// Windows has no permission bits (the code runs on Linux; its tests on
	// every system).
	if fi, err := os.Stat(RecentPath(dirs)); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, %v; want 0600", fi.Mode(), err)
	}
	// Adding it again counts it and moves its times.
	later := xbelNow.Add(time.Hour)
	if err := LinuxAddRecent(dirs, app, "/home/ana/notes/a b#1.md", "text/markdown", later); err != nil {
		t.Fatal(err)
	}
	got := readXBEL(t, dirs)
	if strings.Count(got, "<bookmark ") != 1 || !strings.Contains(got, `count="2"`) ||
		!strings.Contains(got, `added="2026-11-19T10:20:30.123456Z" modified="2026-11-19T11:20:30.123456Z" visited="2026-11-19T11:20:30.123456Z"`) {
		t.Errorf("added again:\n%s", got)
	}
}

func TestXBELMerge(t *testing.T) {
	dirs := xbelDirs(t)
	if err := os.MkdirAll(dirs.DataHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RecentPath(dirs), []byte(gtkXBEL), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file another app opened gets the app beside it; a new file a
	// bookmark of its own. The other app's elements stay as they were.
	if err := LinuxAddRecent(dirs, notesApp, "/home/ana/notes/list.md", "text/markdown", xbelNow); err != nil {
		t.Fatal(err)
	}
	if err := LinuxAddRecent(dirs, notesApp, "/home/ana/notes/new.md", "text/markdown", xbelNow); err != nil {
		t.Fatal(err)
	}
	got := readXBEL(t, dirs)
	for _, s := range []string{
		`<bookmark href="file:///home/ana/Pictures/cat%20(1).png" added="2026-11-18T09:00:00.000001Z"`,
		"        <bookmark:groups>\n          <bookmark:group>Graphics</bookmark:group>\n        </bookmark:groups>\n",
		`<bookmark:application name="Image Viewer" exec="&apos;loupe %u&apos;" modified="2026-11-18T09:00:00.000001Z" count="2"/>`,
		"        <bookmark:private/>\n",
		`<bookmark:application name="org.gnome.TextEditor" exec="&apos;gnome-text-editor %u&apos;" timestamp="1795334400" count="1"/>` + "\n" +
			`          <bookmark:application name="com.example.Notes" exec="&apos;/opt/notes/notes %u&apos;" modified="2026-11-19T10:20:30.123456Z" count="1"/>` + "\n",
		`<bookmark href="file:///home/ana/notes/list.md" added="2026-11-17T08:00:00.000000Z" modified="2026-11-19T10:20:30.123456Z"`,
		`<bookmark href="file:///home/ana/notes/new.md"`,
	} {
		if !strings.Contains(got, s) {
			t.Errorf("merged file lacks %q:\n%s", s, got)
		}
	}
	recent, err := LinuxRecent(dirs, notesApp.ID)
	if err != nil || !slices.Equal(recent, []string{"/home/ana/notes/list.md", "/home/ana/notes/new.md"}) {
		t.Errorf("LinuxRecent = %v, %v", recent, err)
	}

	// Clearing removes the app's application elements, and the bookmark
	// only the app had; the other apps' stay.
	changed, err := LinuxClearRecent(dirs, notesApp, xbelNow)
	if !changed || err != nil {
		t.Fatalf("LinuxClearRecent = %v, %v", changed, err)
	}
	got = readXBEL(t, dirs)
	if strings.Contains(got, "com.example.Notes") || strings.Contains(got, "new.md") ||
		!strings.Contains(got, "list.md") || !strings.Contains(got, "gnome-text-editor") || !strings.Contains(got, "cat%20(1).png") {
		t.Errorf("cleared:\n%s", got)
	}
	// The kept bookmark reads as GLib wrote it, but for its new times.
	if !strings.Contains(got, "          <bookmark:application name=\"org.gnome.TextEditor\" exec=\"&apos;gnome-text-editor %u&apos;\" timestamp=\"1795334400\" count=\"1\"/>\n        </bookmark:applications>\n") {
		t.Errorf("the other app's applications element changed:\n%s", got)
	}
	before := readXBEL(t, dirs)
	if changed, err := LinuxClearRecent(dirs, notesApp, xbelNow); changed || err != nil || readXBEL(t, dirs) != before {
		t.Errorf("clearing again = %v, %v (or it wrote)", changed, err)
	}
}

func TestXBELLimits(t *testing.T) {
	dirs := xbelDirs(t)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<xbel version="1.0" xmlns:bookmark="http://www.freedesktop.org/standards/desktop-bookmarks" xmlns:mime="http://www.freedesktop.org/standards/shared-mime-info">` + "\n")
	// 1000 recent items of another app, the oldest first, and one older
	// than GTK's 30 days.
	old := xbelNow.Add(-31 * 24 * time.Hour).Format(xbelTimeLayout)
	fmt.Fprintf(&b, "  <bookmark href=\"file:///old\" added=%q modified=%q visited=%q/>\n", old, old, old)
	for i := range recentMaxItems {
		at := xbelNow.Add(-time.Duration(recentMaxItems-i) * time.Minute).Format(xbelTimeLayout)
		fmt.Fprintf(&b, "  <bookmark href=\"file:///f%d\" added=%q modified=%q visited=%q/>\n", i, at, at, at)
	}
	b.WriteString("</xbel>\n")
	if err := os.MkdirAll(dirs.DataHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RecentPath(dirs), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LinuxAddRecent(dirs, notesApp, "/home/ana/new.md", "text/markdown", xbelNow); err != nil {
		t.Fatal(err)
	}
	got := readXBEL(t, dirs)
	if n := strings.Count(got, "<bookmark "); n != recentMaxItems {
		t.Errorf("%d bookmarks, want %d", n, recentMaxItems)
	}
	if strings.Contains(got, "file:///old\"") || strings.Contains(got, "file:///f0\"") || !strings.Contains(got, "file:///f1\"") || !strings.Contains(got, "new.md") {
		t.Error("the limits did not drop the item older than 30 days, then the oldest one")
	}
}

func TestXBELBadFile(t *testing.T) {
	dirs := xbelDirs(t)
	if err := os.MkdirAll(dirs.DataHome, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<xbel><bookmark></xbel>", "not xml", "<other/>"} {
		if err := os.WriteFile(RecentPath(dirs), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := LinuxAddRecent(dirs, notesApp, "/home/ana/a.md", "text/markdown", xbelNow); err == nil {
			t.Errorf("AddRecent over %q succeeded", bad)
		}
		if got := readXBEL(t, dirs); got != bad {
			t.Errorf("a file that does not parse was changed to %q", got)
		}
	}
}

func TestXBELURIs(t *testing.T) {
	for path, want := range map[string]string{
		"/home/ana/a b.md":         "file:///home/ana/a%20b.md",
		"/tmp/!$&'()*+,-.:=@_~x":   "file:///tmp/!$&'()*+,-.:=@_~x",
		"/tmp/q?#%;[]\"<>":         "file:///tmp/q%3F%23%25%3B%5B%5D%22%3C%3E",
		"/home/ana/año/ñandú.note": "file:///home/ana/a%C3%B1o/%C3%B1and%C3%BA.note",
	} {
		got := fileURI(path)
		if got != want {
			t.Errorf("fileURI(%q) = %q, want %q", path, got, want)
		}
		if back, ok := filePathOf(got); !ok || back != path {
			t.Errorf("filePathOf(%q) = %q, %v", got, back, ok)
		}
	}
	if p, ok := filePathOf("file://localhost/a%21b"); !ok || p != "/a!b" {
		t.Errorf("localhost URI = %q, %v", p, ok)
	}
	for _, bad := range []string{"https://x/a", "file://server/a", "trash:///a"} {
		if _, ok := filePathOf(bad); ok {
			t.Errorf("filePathOf(%q) took it", bad)
		}
	}
	if got := appCommand(platform.HandlerApp{Exec: "/opt/notes", Args: []string{"--x", "it's 100%"}}); got != `/opt/notes --x 'it'\''s 100%%'` {
		t.Errorf("appCommand = %s", got)
	}
}

func TestRecentMIMEType(t *testing.T) {
	for _, c := range []struct {
		path string
		dir  bool
		want string
	}{
		{"/a/b.png", false, "image/png"},
		{"/a/b.html", false, "text/html"},
		{"/a/b.unknownext", false, "application/octet-stream"},
		{"/a/b", true, "inode/directory"},
	} {
		if got := RecentMIMEType(c.path, c.dir); got != c.want {
			t.Errorf("RecentMIMEType(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
