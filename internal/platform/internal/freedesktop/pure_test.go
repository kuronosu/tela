//go:build linux

package freedesktop

import (
	"image"
	"image/color"
	"net/url"
	"path"
	"slices"
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// pathURI converts an absolute path to a file: URI, as portals do.
func pathURI(p string) string {
	u := url.URL{Scheme: "file", Path: path.Clean(p)}
	return u.String()
}

func TestURIPath(t *testing.T) {
	for _, tc := range []struct {
		uri, want string
	}{
		{"file:///home/ana/notes.txt", "/home/ana/notes.txt"},
		{"file:///home/ana/My%20Files/a%23b%3Fc.txt", "/home/ana/My Files/a#b?c.txt"},
		{"file:///tmp/%C3%B1and%C3%BA+plus%25.png", "/tmp/ñandú+plus%.png"},
		{"file://localhost/etc/hosts", "/etc/hosts"},
		{"FILE:///x", "/x"},
		{"file:///run/user/1000/doc/abc123/report.pdf", "/run/user/1000/doc/abc123/report.pdf"},
	} {
		got, err := uriPath(tc.uri)
		if err != nil || got != tc.want {
			t.Errorf("uriPath(%q) = %q, %v; want %q", tc.uri, got, err, tc.want)
		}
	}
	for _, uri := range []string{
		"https://example.com/a.txt",
		"file://otherhost/a.txt",
		"file:relative.txt",
		"file://",
		"file:///a%00b",
		"file:///a%zz",
		"",
	} {
		if got, err := uriPath(uri); err == nil {
			t.Errorf("uriPath(%q) = %q, nil; want an error", uri, got)
		}
	}
}

func TestURIPathRoundTrip(t *testing.T) {
	for _, p := range []string{"/a", "/a b/c", "/ñ/€/日本.txt", "/%41/#/?/;/&/=", "/x+y/[z]"} {
		uri := pathURI(p)
		got, err := uriPath(uri)
		if err != nil || got != p {
			t.Errorf("uriPath(pathURI(%q) = %q) = %q, %v", p, uri, got, err)
		}
	}
	paths, err := uriPaths([]string{pathURI("/a"), pathURI("/b c")})
	if err != nil || !slices.Equal(paths, []string{"/a", "/b c"}) {
		t.Errorf("uriPaths = %q, %v", paths, err)
	}
	if paths, err := uriPaths([]string{pathURI("/a"), "http://x/"}); err == nil {
		t.Errorf("uriPaths with a non-file URI = %q, nil; want an error", paths)
	}
}

func TestRequestPath(t *testing.T) {
	got := requestPath(":1.42", "tela7")
	if want := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_42/tela7"); got != want {
		t.Errorf("requestPath = %q, want %q", got, want)
	}
	if !got.IsValid() {
		t.Errorf("requestPath %q is not a valid object path", got)
	}
}

func TestArgbPixmap(t *testing.T) {
	img := image.NewNRGBA(image.Rect(10, 20, 12, 21)) // 2×1, offset origin
	img.SetNRGBA(10, 20, color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0x80})
	img.SetNRGBA(11, 20, color.NRGBA{R: 0xff, G: 0x00, B: 0x7f, A: 0xff})
	p := argbPixmap(img)
	if p.Width != 2 || p.Height != 1 {
		t.Fatalf("size %d×%d, want 2×1", p.Width, p.Height)
	}
	want := []byte{0x80, 0x11, 0x22, 0x33, 0xff, 0xff, 0x00, 0x7f}
	if !slices.Equal(p.Data, want) {
		t.Errorf("data % x, want % x (ARGB, network byte order, not premultiplied)", p.Data, want)
	}

	// A premultiplied image is converted to straight alpha first.
	rgba := image.NewRGBA(image.Rect(0, 0, 1, 1))
	rgba.SetRGBA(0, 0, color.RGBA{R: 0x80, A: 0x80})
	if got := argbPixmap(rgba).Data; !slices.Equal(got, []byte{0x80, 0xff, 0, 0}) {
		t.Errorf("premultiplied (128,0,0,128) → % x, want 80 ff 00 00", got)
	}
}

func TestTrayPixmaps(t *testing.T) {
	sq := func(n int) image.Image { return image.NewNRGBA(image.Rect(0, 0, n, n)) }
	sides := func(ps []pixmap) []int32 {
		var out []int32
		for _, p := range ps {
			out = append(out, p.Width)
		}
		return out
	}
	if got := sides(trayPixmaps([]image.Image{sq(64), sq(16), sq(512), sq(32)})); !slices.Equal(got, []int32{16, 32, 64}) {
		t.Errorf("sizes %v, want [16 32 64] (sorted, larger than 256 dropped)", got)
	}
	if got := sides(trayPixmaps([]image.Image{sq(1024), sq(512)})); !slices.Equal(got, []int32{512}) {
		t.Errorf("only large images: sizes %v, want the smallest, [512]", got)
	}
	a := trayPixmaps([]image.Image{sq(16)})
	if !equalPixmaps(a, trayPixmaps([]image.Image{sq(16)})) || equalPixmaps(a, trayPixmaps([]image.Image{sq(24)})) {
		t.Error("equalPixmaps compares sizes and pixels")
	}
}

func TestNotificationImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(5, 5, 8, 7)) // 3×2
	img.SetRGBA(5, 5, color.RGBA{R: 0x80, G: 0, B: 0, A: 0x80})
	d := notificationImage(img)
	if d.Width != 3 || d.Height != 2 || d.Rowstride != 12 || !d.HasAlpha || d.BitsPerSample != 8 || d.Channels != 4 || len(d.Data) != 24 {
		t.Fatalf("image-data %+v", d)
	}
	if got := d.Data[:4]; !slices.Equal(got, []byte{0xff, 0, 0, 0x80}) {
		t.Errorf("first pixel % x, want ff 00 00 80 (straight alpha)", got)
	}
	if sig := dbus.SignatureOf(d).String(); sig != "(iiibiiay)" {
		t.Errorf("signature %s, want (iiibiiay)", sig)
	}
	if sig := dbus.SignatureOf([]pixmap{}).String(); sig != "a(iiay)" {
		t.Errorf("pixmap signature %s, want a(iiay)", sig)
	}
	if sig := dbus.SignatureOf(toolTip{}).String(); sig != "(sa(iiay)ss)" {
		t.Errorf("tooltip signature %s, want (sa(iiay)ss)", sig)
	}
}

var testMenu = []platform.MenuItem{
	{ID: 7, Label: "Open_File"},
	{Separator: true, Label: "ignored"},
	{ID: 9, Label: "Dark mode", Checkable: true, Checked: true},
	{ID: 10, Label: "Muted", Checkable: true},
	{Label: "More", Submenu: []platform.MenuItem{
		{ID: 11, Label: "Disabled", Disabled: true},
		{ID: 12, Label: "About"},
	}},
}

func prop[T any](t *testing.T, n *menuNode, name string) T {
	t.Helper()
	v, ok := n.props[name]
	if !ok {
		t.Fatalf("item %d has no %q property (props %v)", n.id, name, n.props)
	}
	x, ok := v.Value().(T)
	if !ok {
		t.Fatalf("item %d: %q is %T, want %T", n.id, name, v.Value(), x)
	}
	return x
}

func TestMenuLayout(t *testing.T) {
	var ids menuIDs
	root, nodes := ids.build(testMenu)
	if root.id != 0 || prop[string](t, root, "children-display") != "submenu" || len(root.children) != 5 {
		t.Fatalf("root: id %d, props %v, %d children", root.id, root.props, len(root.children))
	}
	open, sep, dark, muted, more := root.children[0], root.children[1], root.children[2], root.children[3], root.children[4]
	if prop[string](t, open, "label") != "Open__File" || !prop[bool](t, open, "enabled") || open.item != 7 {
		t.Errorf("Open_File: %v, item %d", open.props, open.item)
	}
	if _, ok := open.props["toggle-type"]; ok {
		t.Errorf("a plain item has a toggle-type: %v", open.props)
	}
	if prop[string](t, sep, "type") != "separator" || sep.item != 0 {
		t.Errorf("separator: %v, item %d", sep.props, sep.item)
	}
	if _, ok := sep.props["label"]; ok {
		t.Errorf("separator has a label: %v", sep.props)
	}
	if prop[string](t, dark, "toggle-type") != "checkmark" || prop[int32](t, dark, "toggle-state") != 1 {
		t.Errorf("checked item: %v", dark.props)
	}
	if prop[int32](t, muted, "toggle-state") != 0 {
		t.Errorf("unchecked item: %v", muted.props)
	}
	if prop[string](t, more, "children-display") != "submenu" || more.item != 0 || len(more.children) != 2 {
		t.Fatalf("submenu: %v, item %d, %d children", more.props, more.item, len(more.children))
	}
	disabled, about := more.children[0], more.children[1]
	if prop[bool](t, disabled, "enabled") || disabled.item != 0 {
		t.Errorf("disabled item: %v, item %d (want not activatable)", disabled.props, disabled.item)
	}
	if about.item != 12 {
		t.Errorf("about: item %d", about.item)
	}
	if len(nodes) != 8 {
		t.Errorf("%d nodes, want 8 (root and 7 items)", len(nodes))
	}
	seen := map[int32]bool{}
	for id, n := range nodes {
		if n.id != id || seen[id] {
			t.Errorf("node index: id %d → node %d", id, n.id)
		}
		seen[id] = true
	}

	// Activatable items keep their ids across builds; the others do not.
	root2, _ := ids.build(slices.Insert(slices.Clone(testMenu), 0, platform.MenuItem{ID: 99, Label: "New"}))
	if root2.children[1].id != open.id || root2.children[3].id != dark.id {
		t.Errorf("ids changed across builds: %d→%d, %d→%d", open.id, root2.children[1].id, dark.id, root2.children[3].id)
	}
	if root2.children[2].id == sep.id || root2.children[5].id == more.id {
		t.Error("separators and submenus reuse old ids")
	}
}

func TestMenuLayoutDepthAndFilter(t *testing.T) {
	var ids menuIDs
	root, _ := ids.build(testMenu)
	all := root.layout(-1, nil)
	if len(all.Children) != 5 {
		t.Fatalf("%d children, want 5", len(all.Children))
	}
	more, ok := all.Children[4].Value().(layoutItem)
	if !ok || len(more.Children) != 2 {
		t.Fatalf("submenu layout %#v", all.Children[4].Value())
	}
	if top := root.layout(1, nil); len(top.Children) != 5 {
		t.Errorf("depth 1: %d children", len(top.Children))
	} else if sub := top.Children[4].Value().(layoutItem); len(sub.Children) != 0 {
		t.Errorf("depth 1: the submenu has %d children, want 0", len(sub.Children))
	}
	if none := root.layout(0, nil); len(none.Children) != 0 {
		t.Errorf("depth 0: %d children", len(none.Children))
	}
	labels := root.layout(-1, []string{"label"})
	first := labels.Children[0].Value().(layoutItem)
	if len(first.Props) != 1 || first.Props["label"].Value() != "Open__File" {
		t.Errorf("filtered props %v", first.Props)
	}
	if sig := dbus.SignatureOf(all).String(); sig != "(ia{sv}av)" {
		t.Errorf("layout signature %s, want (ia{sv}av)", sig)
	}
}

func TestEqualMenus(t *testing.T) {
	clone := cloneMenu(testMenu)
	if !equalMenus(testMenu, clone) {
		t.Error("a menu differs from its clone")
	}
	clone[4].Submenu[1].Label = "About…"
	if equalMenus(testMenu, clone) {
		t.Error("a changed submenu label is not a change")
	}
	if testMenu[4].Submenu[1].Label != "About" {
		t.Error("cloneMenu shares submenus")
	}
	if !equalMenus(nil, []platform.MenuItem{}) {
		t.Error("nil and empty menus differ")
	}
}

func TestEscapeMarkup(t *testing.T) {
	if got := escapeMarkup(`a < b & c > "d"`); got != `a &lt; b &amp; c &gt; "d"` {
		t.Errorf("escapeMarkup = %q", got)
	}
}
