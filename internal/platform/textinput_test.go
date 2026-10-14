package platform

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestTextInputStateEqualClone(t *testing.T) {
	a := &TextInputState{CursorArea: Rect{1, 2, 0, 18}, Purpose: TextPurposeEmail, Surrounding: &SurroundingText{"ab", 1, 1}}
	b := a.Clone()
	if !a.Equal(b) || !b.Equal(a) {
		t.Errorf("a clone is not equal")
	}
	b.Surrounding.Text = "ac"
	if a.Surrounding.Text != "ab" {
		t.Errorf("Clone shares Surrounding")
	}
	if a.Equal(b) {
		t.Errorf("different surrounding text compares equal")
	}
	var nilState *TextInputState
	if !nilState.Equal(nil) || nilState.Equal(a) || a.Equal(nil) || nilState.Clone() != nil {
		t.Errorf("nil handling")
	}
	c := a.Clone()
	c.Surrounding = nil
	if a.Equal(c) || c.Equal(a) {
		t.Errorf("state without surrounding text compares equal to one with")
	}
	d := a.Clone()
	d.Multiline = true
	if a.Equal(d) {
		t.Errorf("Multiline ignored")
	}
	for p := TextPurposeNormal; p.Valid(); p++ {
		if strings.Contains(p.String(), "invalid") {
			t.Errorf("TextPurpose %d has no name", p)
		}
	}
	if TextPurpose(200).Valid() || TextPurpose(200).String() != "TextPurpose(invalid)" {
		t.Errorf("invalid purpose")
	}
}

func TestCheckTextInputState(t *testing.T) {
	CheckTextInputState(nil)
	CheckTextInputState(&TextInputState{})
	CheckTextInputState(&TextInputState{CursorArea: Rect{-5, 3, 0, 20}, Surrounding: &SurroundingText{"é", 2, 0}})
	nan := float32(math.NaN())
	for _, tc := range []struct {
		s    TextInputState
		want string
	}{
		{TextInputState{CursorArea: Rect{nan, 0, 0, 0}}, "finite"},
		{TextInputState{CursorArea: Rect{0, float32(math.Inf(1)), 0, 0}}, "finite"},
		{TextInputState{CursorArea: Rect{0, 0, -1, 10}}, "negative"},
		{TextInputState{Purpose: 99}, "Purpose"},
		{TextInputState{Surrounding: &SurroundingText{Text: "\xff"}}, "UTF-8"},
		{TextInputState{Surrounding: &SurroundingText{Text: "ab", Cursor: 3}}, "outside"},
		{TextInputState{Surrounding: &SurroundingText{Text: "ab", Anchor: -1}}, "outside"},
		{TextInputState{Surrounding: &SurroundingText{Text: "é", Cursor: 1}}, "inside a UTF-8"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Errorf("CheckTextInputState(%+v): recover() = %v, want %q", tc.s, r, tc.want)
				}
			}()
			CheckTextInputState(&tc.s)
		}()
	}
}

func TestIMEHelpers(t *testing.T) {
	if got := SanitizeIMEText("a\xffb\x00c"); got != "a�bc" {
		t.Errorf("SanitizeIMEText = %q", got)
	}
	if got := SanitizeIMEText("ok"); got != "ok" {
		t.Errorf("SanitizeIMEText = %q", got)
	}
	for _, tc := range []struct {
		s          string
		start, end int
		ws, we     int
	}{
		{"", 3, 4, 0, 0},
		{"abc", -1, -1, -1, -1},
		{"abc", -1, 2, -1, -1},
		{"abc", 2, 9, 2, 3},
		{"abc", 2, 1, 1, 2},
		{"aé", 2, 2, 1, 1},
	} {
		if s, e := SanitizePreeditCursor(tc.s, tc.start, tc.end); s != tc.ws || e != tc.we {
			t.Errorf("SanitizePreeditCursor(%q, %d, %d) = %d, %d, want %d, %d", tc.s, tc.start, tc.end, s, e, tc.ws, tc.we)
		}
	}
	for _, tc := range []struct {
		e    IMEEvent
		want string
	}{
		{IMEEvent{}, ""},
		{IMEEvent{Commit: "x", Preedit: "かな", CursorStart: 3, CursorEnd: 6}, ""},
		{IMEEvent{Preedit: "か", CursorStart: -1, CursorEnd: -1}, ""},
		{IMEEvent{DeleteBefore: -1}, "negative"},
		{IMEEvent{Commit: "\xff"}, "UTF-8"},
		{IMEEvent{Preedit: "a\x00"}, "NUL"},
		{IMEEvent{CursorStart: 1}, "empty Preedit"},
		{IMEEvent{Preedit: "ab", CursorStart: -1, CursorEnd: 1}, "hidden"},
		{IMEEvent{Preedit: "ab", CursorStart: 2, CursorEnd: 1}, "ordered"},
		{IMEEvent{Preedit: "ab", CursorStart: 0, CursorEnd: 3}, "ordered"},
		{IMEEvent{Preedit: "か", CursorStart: 1, CursorEnd: 1}, "inside"},
	} {
		if got := CheckIMEEvent(tc.e); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("CheckIMEEvent(%+v) = %q, want %q", tc.e, got, tc.want)
		}
	}
	var _ Event = IMEEvent{}
	var _ Event = IMESessionEvent{}
	if s := (Rect{1, 2, 3, 4}).String(); s != "(1,2 3x4)" {
		t.Errorf("Rect.String() = %q", s)
	}
}
