package platform

import (
	"fmt"
	"strings"
	"testing"
)

// AutoAccessKeys gives every item of a menu level a key, the Win32 bar's
// rule (ADR-234, ADR-252, ADR-260).
func TestAutoAccessKeys(t *testing.T) {
	items := []MenuItem{
		{Label: "File"}, {Label: "Format"}, {Separator: true}, {Label: "Fit", Disabled: true},
		{Label: "コピー", Role: RoleCopy}, {Label: "編集", AccessKey: 'e'}, {Label: "ファイル"}, {Label: "Window", AccessKey: 'd'},
	}
	var got []string
	for i, m := range AutoAccessKeys(items) {
		switch {
		case m.At >= 0:
			got = append(got, items[i].Label[:m.At]+"&"+items[i].Label[m.At:])
		case m.Key != 0:
			got = append(got, fmt.Sprintf("%s(&%c)", items[i].Label, m.Key))
		default:
			got = append(got, items[i].Label+"-")
		}
	}
	// Copy's prolonged sound mark is no key of its own: the role's C.
	want := "&File F&ormat - Fit- コピー(&C) 編集(&E) ファイル(&A) Win&dow"
	if s := strings.Join(got, " "); s != want {
		t.Errorf("AutoAccessKeys:\n got %s\nwant %s", s, want)
	}
	if AutoAccessKeys(nil) != nil {
		t.Error("nil for an empty menu")
	}
	for _, r := range []rune{'ー', '、', 'ｱ', '漢'} {
		if CheckAccessKey(r) {
			t.Errorf("CheckAccessKey(%q)", r)
		}
	}
}
