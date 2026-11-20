package assoc

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The recent documents on Linux (platform contract v16, ADR-298):
// $XDG_DATA_HOME/recently-used.xbel, the file GTK's GtkRecentManager (and
// so every GTK file chooser, Nautilus' Recent, GNOME Shell's search) and
// KDE's file dialogs read and write, in the format of GLib's
// GBookmarkFile (checked against GLib 2.80's gbookmarkfile.c):
//
//	<bookmark href="file:///home/ana/notes/a.md" added="…" modified="…" visited="…">
//	  <info>
//	    <metadata owner="http://freedesktop.org">
//	      <mime:mime-type type="text/markdown"/>
//	      <bookmark:applications>
//	        <bookmark:application name="com.example.Notes" exec="&apos;/usr/bin/notes %u&apos;" modified="…" count="3"/>
//	      </bookmark:applications>
//	    </metadata>
//	  </info>
//	</bookmark>
//
// Times are ISO 8601 in UTC with microseconds, as GLib 2.66 and later
// write them (an application's older "timestamp" attribute, seconds, is
// read and replaced); href is the file's URI escaped as g_filename_to_uri
// does; exec is the command shell-quoted as a whole, as
// g_bookmark_file_set_application_info stores it (GTK writes 'prgname
// %u'). The application is named by the app's ID, which is how clearing
// finds the app's own entries.
//
// Other applications' bookmarks are kept as they are: the file is parsed
// into a tree that keeps every element, attribute, comment and the
// whitespace between them, only the app's bookmark and application
// elements change, and the rest is written back unchanged. It is written
// atomically (a temporary file renamed over it), after checking that no
// other program changed it since it was read; when one did, the change is
// made again on what it wrote. GTK's limits are applied as GTK applies
// them on its own writes (gtkrecentmanager.c): items not modified for 30
// days (gtk-recent-files-max-age's default) are dropped, then the oldest
// beyond 1000 (MAX_LIST_SIZE). A file that does not parse is an error and
// is left alone (GTK would start a new one; another program's data is
// not Tela's to drop).

const (
	recentFile        = "recently-used.xbel"
	bookmarkNS        = "http://www.freedesktop.org/standards/desktop-bookmarks"
	xbelMIMENS        = "http://www.freedesktop.org/standards/shared-mime-info"
	metadataOwner     = "http://freedesktop.org"
	recentMaxItems    = 1000
	recentMaxAge      = 30 * 24 * time.Hour
	recentWriteTries  = 4
	xbelTimeLayout    = "2006-01-02T15:04:05.000000Z"
	recentDefaultMIME = "application/octet-stream"
)

// RecentPath returns the recently-used.xbel of dirs.
func RecentPath(dirs LinuxDirs) string { return filepath.Join(dirs.DataHome, recentFile) }

// RecentMIMEType returns the MIME type recorded for a recent file: a
// folder's inode/directory, else the type of its extension
// (mime.TypeByExtension, without parameters), else
// application/octet-stream.
func RecentMIMEType(path string, isDir bool) string {
	if isDir {
		return "inode/directory"
	}
	t := mime.TypeByExtension(filepath.Ext(path))
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = t[:i]
	}
	if t = strings.TrimSpace(t); t == "" {
		return recentDefaultMIME
	}
	return t
}

// LinuxAddRecent adds path (absolute) of the MIME type typ to the recent
// documents of dirs for app at now: a new bookmark, or the existing one
// with its modified and visited times and the app's count updated.
func LinuxAddRecent(dirs LinuxDirs, app platform.HandlerApp, path, typ string, now time.Time) error {
	platform.CheckHandlerApp("AddRecentDocument", app, true)
	clean, err := platform.CheckRecentPath(path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(dirs.DataHome) {
		return fmt.Errorf("assoc: LinuxDirs.DataHome %q is not an absolute path ($XDG_DATA_HOME)", dirs.DataHome)
	}
	_, err = updateRecent(RecentPath(dirs), func(doc *xbelDoc) bool {
		doc.add(app, clean, typ, now)
		return true
	}, now)
	return err
}

// LinuxClearRecent removes the app's application element from every
// bookmark of the recent documents, and the bookmarks left with none. It
// reports whether it wrote anything.
func LinuxClearRecent(dirs LinuxDirs, app platform.HandlerApp, now time.Time) (bool, error) {
	platform.CheckHandlerApp("ClearRecentDocuments", app, false)
	if !filepath.IsAbs(dirs.DataHome) {
		return false, fmt.Errorf("assoc: LinuxDirs.DataHome %q is not an absolute path ($XDG_DATA_HOME)", dirs.DataHome)
	}
	return updateRecent(RecentPath(dirs), func(doc *xbelDoc) bool { return doc.clear(app.ID) }, now)
}

// LinuxRecent returns the paths of the app's recent documents in the
// file, most recently modified first (for tests and status).
func LinuxRecent(dirs LinuxDirs, appID string) ([]string, error) {
	raw, err := readMaybe(RecentPath(dirs))
	if err != nil || raw == nil {
		return nil, err
	}
	doc, err := parseXBEL(raw)
	if err != nil {
		return nil, err
	}
	type item struct {
		path string
		at   time.Time
	}
	var items []item
	for _, bm := range doc.bookmarks() {
		if doc.appOf(bm, appID) == nil {
			continue
		}
		if p, ok := filePathOf(bm.attr("href")); ok {
			items = append(items, item{p, bookmarkTime(bm)})
		}
	}
	slices.SortStableFunc(items, func(a, b item) int { return b.at.Compare(a.at) })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out, nil
}

// updateRecent reads the file, lets change edit it (it reports whether
// it changed anything), applies GTK's limits and writes it back if its
// bytes changed, again on a newer file when another program wrote one
// meanwhile.
func updateRecent(path string, change func(*xbelDoc) bool, now time.Time) (bool, error) {
	for range recentWriteTries {
		raw, err := readMaybe(path)
		if err != nil {
			return false, fmt.Errorf("assoc: %w", err)
		}
		doc := newXBEL()
		if raw != nil {
			if doc, err = parseXBEL(raw); err != nil {
				return false, fmt.Errorf("assoc: %s: %w (the file is left as it is)", path, err)
			}
		}
		changed := change(doc)
		if doc.clamp(now) {
			changed = true
		}
		if !changed {
			return false, nil
		}
		data := doc.bytes()
		if raw != nil && bytes.Equal(data, raw) {
			return false, nil
		}
		ok, err := replaceIfUnchanged(path, data, raw)
		if err != nil {
			return false, fmt.Errorf("assoc: %w", err)
		}
		if ok {
			return true, nil
		}
	}
	return false, fmt.Errorf("assoc: %s kept changing while it was written; try again", path)
}

// replaceIfUnchanged writes data to a temporary file beside path and
// renames it over path, unless path no longer holds old (nil: missing),
// in which case it removes the temporary file and returns false. An
// existing file keeps its permissions; a new one gets 0600 (it lists the
// user's files).
func replaceIfUnchanged(path string, data, old []byte) (bool, error) {
	path, err := resolveLink(path)
	if err != nil {
		return false, err
	}
	perm := fs.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return false, err
	}
	tmp := f.Name()
	renamed := false
	defer func() {
		if !renamed {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return false, err
	}
	if err := f.Chmod(perm); err != nil {
		return false, err
	}
	if err := f.Sync(); err != nil {
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	cur, err := readMaybe(path)
	if err != nil {
		return false, err
	}
	if (cur == nil) != (old == nil) || !bytes.Equal(cur, old) {
		return false, nil
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, err
	}
	renamed = true
	return true, nil
}

// --- the document ---

type xkind uint8

const (
	xElem xkind = iota
	xText
	xComment
	xProcInst
	xDirective
)

// xnode is a node of an XML document as written: element and attribute
// names keep their prefixes ("bookmark:application"), text keeps its
// whitespace.
type xnode struct {
	kind  xkind
	name  string // element name; a processing instruction's target
	attrs []xattr
	kids  []*xnode
	text  string // text, comment, instruction, directive
}

type xattr struct{ name, value string }

func (n *xnode) attr(name string) string {
	for _, a := range n.attrs {
		if a.name == name {
			return a.value
		}
	}
	return ""
}

func (n *xnode) setAttr(name, value string) {
	for i := range n.attrs {
		if n.attrs[i].name == name {
			n.attrs[i].value = value
			return
		}
	}
	n.attrs = append(n.attrs, xattr{name, value})
}

func (n *xnode) delAttr(name string) {
	n.attrs = slices.DeleteFunc(n.attrs, func(a xattr) bool { return a.name == name })
}

// elems returns the child elements named name.
func (n *xnode) elems(name string) []*xnode {
	var out []*xnode
	for _, k := range n.kids {
		if k.kind == xElem && k.name == name {
			out = append(out, k)
		}
	}
	return out
}

func (n *xnode) elem(name string) *xnode {
	for _, k := range n.kids {
		if k.kind == xElem && k.name == name {
			return k
		}
	}
	return nil
}

// insert appends child to n as the depth'th level of the document's
// indentation (two spaces a level, as GLib writes): after n's last
// element, before the whitespace that indents n's end tag.
func (n *xnode) insert(child *xnode, depth int) {
	ind := "\n" + strings.Repeat("  ", depth)
	if len(n.kids) == 0 {
		n.kids = []*xnode{{kind: xText, text: ind}, child, {kind: xText, text: "\n" + strings.Repeat("  ", depth-1)}}
		return
	}
	at := len(n.kids)
	if last := n.kids[at-1]; last.kind == xText && strings.TrimSpace(last.text) == "" {
		at--
	}
	n.kids = slices.Insert(n.kids, at, &xnode{kind: xText, text: ind}, child)
}

// remove removes child from n, with the whitespace that indents it.
func (n *xnode) remove(child *xnode) {
	i := slices.Index(n.kids, child)
	if i < 0 {
		return
	}
	j := i
	if i > 0 && n.kids[i-1].kind == xText && strings.TrimSpace(n.kids[i-1].text) == "" {
		i--
	}
	n.kids = slices.Delete(n.kids, i, j+1)
}

// xbelDoc is a recently-used.xbel: its nodes, its root, and the prefixes
// its root binds the bookmark and MIME namespaces to.
type xbelDoc struct {
	nodes      []*xnode
	root       *xnode
	bm, mimeNS string // prefixes: "bookmark", "mime"
}

func newXBEL() *xbelDoc {
	root := &xnode{kind: xElem, name: "xbel", attrs: []xattr{
		{"version", "1.0"}, {"xmlns:bookmark", bookmarkNS}, {"xmlns:mime", xbelMIMENS},
	}}
	return &xbelDoc{
		nodes: []*xnode{
			{kind: xProcInst, name: "xml", text: `version="1.0" encoding="UTF-8"`},
			{kind: xText, text: "\n"}, root, {kind: xText, text: "\n"},
		},
		root: root, bm: "bookmark", mimeNS: "mime",
	}
}

// parseXBEL parses raw, which must hold one xbel root element.
func parseXBEL(raw []byte) (*xbelDoc, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	doc := &xbelDoc{}
	var stack []*xnode
	add := func(n *xnode) {
		if len(stack) == 0 {
			doc.nodes = append(doc.nodes, n)
		} else {
			p := stack[len(stack)-1]
			p.kids = append(p.kids, n)
		}
	}
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{kind: xElem, name: qname(t.Name)}
			for _, a := range t.Attr {
				n.attrs = append(n.attrs, xattr{qname(a.Name), a.Value})
			}
			if len(stack) == 0 {
				if doc.root != nil {
					return nil, errors.New("more than one root element")
				}
				doc.root = n
			}
			add(n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].name != qname(t.Name) {
				return nil, fmt.Errorf("unexpected end tag </%s>", qname(t.Name))
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("text outside the root element")
			}
			add(&xnode{kind: xText, text: string(t)})
		case xml.Comment:
			add(&xnode{kind: xComment, text: string(t)})
		case xml.ProcInst:
			add(&xnode{kind: xProcInst, name: t.Target, text: string(t.Inst)})
		case xml.Directive:
			add(&xnode{kind: xDirective, text: string(t)})
		}
	}
	switch {
	case len(stack) > 0:
		return nil, fmt.Errorf("unclosed element <%s>", stack[len(stack)-1].name)
	case doc.root == nil || localName(doc.root.name) != "xbel":
		return nil, errors.New("not an XBEL document (no xbel root element)")
	}
	doc.bm = doc.prefix(bookmarkNS, "bookmark", false)
	doc.mimeNS = doc.prefix(xbelMIMENS, "mime", false)
	return doc, nil
}

func qname(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

func localName(n string) string {
	if i := strings.IndexByte(n, ':'); i >= 0 {
		return n[i+1:]
	}
	return n
}

// prefix returns the prefix the root binds ns to; when it binds none,
// def, or a free variant of it, bound to ns when bind is set.
func (d *xbelDoc) prefix(ns, def string, bind bool) string {
	for _, a := range d.root.attrs {
		if strings.HasPrefix(a.name, "xmlns:") && a.value == ns {
			return strings.TrimPrefix(a.name, "xmlns:")
		}
	}
	if !bind {
		return def
	}
	p := def
	for i := 2; d.root.attr("xmlns:"+p) != ""; i++ {
		p = def + strconv.Itoa(i)
	}
	d.root.attrs = append(d.root.attrs, xattr{"xmlns:" + p, ns})
	return p
}

func (d *xbelDoc) bookmarks() []*xnode { return d.root.elems("bookmark") }

// metadata returns the freedesktop metadata element of a bookmark (the
// one GLib reads), creating it (and info) when make is set.
func (d *xbelDoc) metadata(bm *xnode, create bool) *xnode {
	info := bm.elem("info")
	if info != nil {
		for _, m := range info.elems("metadata") {
			if m.attr("owner") == metadataOwner {
				return m
			}
		}
	}
	if !create {
		return nil
	}
	if info == nil {
		info = &xnode{kind: xElem, name: "info"}
		bm.insert(info, 2)
	}
	m := &xnode{kind: xElem, name: "metadata", attrs: []xattr{{"owner", metadataOwner}}}
	info.insert(m, 3)
	return m
}

// appOf returns the application element named name of a bookmark, or nil.
func (d *xbelDoc) appOf(bm *xnode, name string) *xnode {
	m := d.metadata(bm, false)
	if m == nil {
		return nil
	}
	for _, apps := range m.elems(d.bm + ":applications") {
		for _, a := range apps.elems(d.bm + ":application") {
			if a.attr("name") == name {
				return a
			}
		}
	}
	return nil
}

// add records the app's use of path at now.
func (d *xbelDoc) add(app platform.HandlerApp, path, typ string, now time.Time) {
	d.bm = d.prefix(bookmarkNS, d.bm, true)
	d.mimeNS = d.prefix(xbelMIMENS, d.mimeNS, true)
	stamp := now.UTC().Format(xbelTimeLayout)
	var bm *xnode
	for _, b := range d.bookmarks() {
		if p, ok := filePathOf(b.attr("href")); ok && p == path {
			bm = b
			break
		}
	}
	if bm == nil {
		bm = &xnode{kind: xElem, name: "bookmark", attrs: []xattr{
			{"href", fileURI(path)}, {"added", stamp}, {"modified", stamp}, {"visited", stamp},
		}}
		d.root.insert(bm, 1)
	} else {
		bm.setAttr("modified", stamp)
		bm.setAttr("visited", stamp)
	}
	m := d.metadata(bm, true)
	if m.elem(d.mimeNS+":mime-type") == nil {
		m.insert(&xnode{kind: xElem, name: d.mimeNS + ":mime-type", attrs: []xattr{{"type", typ}}}, 4)
	}
	apps := m.elem(d.bm + ":applications")
	if apps == nil {
		apps = &xnode{kind: xElem, name: d.bm + ":applications"}
		m.insert(apps, 4)
	}
	exec := shellQuote(appCommand(app) + " %u")
	if a := d.appOf(bm, app.ID); a != nil {
		count, _ := strconv.Atoi(a.attr("count"))
		a.setAttr("exec", exec)
		a.setAttr("modified", stamp)
		a.delAttr("timestamp")
		a.setAttr("count", strconv.Itoa(max(count, 0)+1))
		return
	}
	apps.insert(&xnode{kind: xElem, name: d.bm + ":application", attrs: []xattr{
		{"name", app.ID}, {"exec", exec}, {"modified", stamp}, {"count", "1"},
	}}, 5)
}

// clear removes the application elements named id, and the bookmarks
// left without applications; it reports whether it removed any.
func (d *xbelDoc) clear(id string) bool {
	changed := false
	for _, bm := range d.bookmarks() {
		a := d.appOf(bm, id)
		if a == nil {
			continue
		}
		changed = true
		m := d.metadata(bm, false)
		left := 0
		for _, apps := range m.elems(d.bm + ":applications") {
			apps.remove(a)
			left += len(apps.elems(d.bm + ":application"))
		}
		if left == 0 {
			d.root.remove(bm)
		}
	}
	return changed
}

// clamp applies GTK's limits: bookmarks not modified within recentMaxAge
// go, then the oldest beyond recentMaxItems. It reports whether it
// removed any.
func (d *xbelDoc) clamp(now time.Time) bool {
	removed := false
	for _, bm := range d.bookmarks() {
		if at := bookmarkTime(bm); !at.IsZero() && now.Sub(at) > recentMaxAge {
			d.root.remove(bm)
			removed = true
		}
	}
	bms := d.bookmarks()
	if len(bms) <= recentMaxItems {
		return removed
	}
	slices.SortStableFunc(bms, func(a, b *xnode) int { return bookmarkTime(a).Compare(bookmarkTime(b)) })
	for _, bm := range bms[:len(bms)-recentMaxItems] {
		d.root.remove(bm)
	}
	return true
}

// bookmarkTime returns when a bookmark was last modified (else added,
// else visited); zero when it says nothing readable.
func bookmarkTime(bm *xnode) time.Time {
	for _, k := range []string{"modified", "added", "visited"} {
		if t, err := time.Parse(time.RFC3339Nano, bm.attr(k)); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (d *xbelDoc) bytes() []byte {
	var b bytes.Buffer
	for _, n := range d.nodes {
		writeNode(&b, n)
	}
	return b.Bytes()
}

func writeNode(b *bytes.Buffer, n *xnode) {
	switch n.kind {
	case xText:
		b.WriteString(escapeXML(n.text, false))
	case xComment:
		b.WriteString("<!--" + n.text + "-->")
	case xProcInst:
		b.WriteString("<?" + n.name)
		if n.text != "" {
			b.WriteString(" " + n.text)
		}
		b.WriteString("?>")
	case xDirective:
		b.WriteString("<!" + n.text + ">")
	case xElem:
		b.WriteString("<" + n.name)
		for _, a := range n.attrs {
			b.WriteString(" " + a.name + `="` + escapeXML(a.value, true) + `"`)
		}
		if len(n.kids) == 0 {
			b.WriteString("/>")
			return
		}
		b.WriteString(">")
		for _, k := range n.kids {
			writeNode(b, k)
		}
		b.WriteString("</" + n.name + ">")
	}
}

// escapeXML escapes text, or an attribute value (quotes and the
// whitespace characters an attribute would normalize too), as GLib's
// g_markup_escape_text does for the quotes.
func escapeXML(s string, attr bool) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case attr && r == '"':
			b.WriteString("&quot;")
		case attr && r == '\'':
			b.WriteString("&apos;")
		case attr && (r == '\n' || r == '\r' || r == '\t'):
			b.WriteString("&#" + strconv.Itoa(int(r)) + ";")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// --- URIs and commands ---

// fileURI returns the file: URI of an absolute path, escaped as GLib's
// g_filename_to_uri does: letters, digits and !$&'()*+,-./:=@_~ are kept,
// every other byte is %XX.
func fileURI(path string) string {
	var b strings.Builder
	b.WriteString("file://")
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("!$&'()*+,-./:=@_~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// filePathOf returns the path of a local file: URI (any host-less or
// localhost form, any escaping).
func filePathOf(href string) (string, bool) {
	u, err := url.Parse(href)
	if err != nil || !strings.EqualFold(u.Scheme, "file") || (u.Host != "" && u.Host != "localhost") || u.Path == "" {
		return "", false
	}
	return filepath.Clean(u.Path), true
}

// appCommand returns the app's command line for a shell: the executable
// and its arguments, each quoted when it needs it, with '%' written "%%"
// (GLib expands %u and %f in it).
func appCommand(app platform.HandlerApp) string {
	parts := []string{shellWord(app.Exec)}
	for _, a := range app.Args {
		parts = append(parts, shellWord(a))
	}
	return strings.ReplaceAll(strings.Join(parts, " "), "%", "%%")
}

// shellWord quotes a word for a POSIX shell when it holds anything but
// letters, digits and -_./=+,:@.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=+,:@") == "" {
		return s
	}
	return shellQuote(s)
}

// shellQuote is GLib's g_shell_quote: the text in single quotes, each
// single quote written '\”.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
