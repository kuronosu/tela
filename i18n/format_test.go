package i18n

import (
	"strings"
	"testing"
	"time"
)

func TestNumbers(t *testing.T) {
	tests := []struct {
		loc  string
		v    float64
		opts []NumberOption
		want string
	}{
		{"en", 1234.5, nil, "1,234.5"},
		{"en-US", 1234567.891, nil, "1,234,567.891"},
		{"es", 1234.5, nil, "1.234,5"},
		{"es", 1234567.891, nil, "1.234.567,891"},
		{"es-MX", 1234.5, nil, "1,234.5"},
		{"de", 1234.5, nil, "1.234,5"},
		{"fr", 1234.5, nil, "1 234,5"},
		{"ru", 1234567, nil, "1 234 567"},
		{"hi", 1234567, nil, "12,34,567"},
		{"pt-BR", 1234.5, nil, "1.234,5"},
		// Arabic: CLDR's default numbering system for ar is arab.
		{"ar", 1234.5, nil, "١٬٢٣٤٫٥"},
		{"fa", 12, nil, "۱۲"},
		{"ja", 1234.5, nil, "1,234.5"},
		{"en", 0.12, []NumberOption{Percent()}, "12%"},
		{"es", 0.12, []NumberOption{Percent()}, "12 %"},
		{"en", 2, []NumberOption{MinFractionDigits(2)}, "2.00"},
		{"es", 2, []NumberOption{MinFractionDigits(2)}, "2,00"},
		{"en", 1.23456, nil, "1.235"},
		{"en", 1.23456, []NumberOption{MaxFractionDigits(1)}, "1.2"},
		{"en", 1.23456, []NumberOption{MaxFractionDigits(5)}, "1.23456"},
		{"en", 1234.5, []NumberOption{NoGrouping()}, "1234.5"},
		{"en", 0.256, []NumberOption{Percent(), MaxFractionDigits(1)}, "25.6%"},
		{"en", -1234.5, nil, "-1,234.5"},
		{"en", 0, nil, "0"},
	}
	for _, tc := range tests {
		if got := For(tc.loc).Number(tc.v, tc.opts...); got != tc.want {
			t.Errorf("%s %v = %q, want %q", tc.loc, tc.v, got, tc.want)
		}
	}
}

func TestDates(t *testing.T) {
	tm := time.Date(2026, 1, 5, 15, 4, 5, 0, time.UTC)
	tests := []struct {
		loc                       string
		short, medium, long, full string
	}{
		{"en", "1/5/26", "Jan 5, 2026", "January 5, 2026", "Monday, January 5, 2026"},
		{"en-GB", "05/01/2026", "5 Jan 2026", "5 January 2026", "Monday, 5 January 2026"},
		{"es", "5/1/26", "5 ene 2026", "5 de enero de 2026", "lunes, 5 de enero de 2026"},
		{"es-MX", "05/01/26", "5 ene 2026", "5 de enero de 2026", "lunes, 5 de enero de 2026"},
		{"de", "05.01.26", "05.01.2026", "5. Januar 2026", "Montag, 5. Januar 2026"},
		{"fr", "05/01/2026", "5 janv. 2026", "5 janvier 2026", "lundi 5 janvier 2026"},
		{"ja", "2026/01/05", "2026/01/05", "2026年1月5日", "2026年1月5日月曜日"},
		{"zh", "2026/1/5", "2026年1月5日", "2026年1月5日", "2026年1月5日星期一"},
		{"ko", "26. 1. 5.", "2026. 1. 5.", "2026년 1월 5일", "2026년 1월 5일 월요일"},
		{"ru", "05.01.2026", "5 янв. 2026 г.", "5 января 2026 г.", "понедельник, 5 января 2026 г."},
		// Arabic uses Arabic-Indic digits and right-to-left marks.
		{"ar", "٥\u200f/١\u200f/٢٠٢٦", "٠٥\u200f/٠١\u200f/٢٠٢٦", "٥ يناير ٢٠٢٦", "الاثنين، ٥ يناير ٢٠٢٦"},
	}
	for _, tc := range tests {
		f := For(tc.loc)
		for _, c := range []struct {
			style DateStyle
			want  string
		}{{Short, tc.short}, {Medium, tc.medium}, {Long, tc.long}, {Full, tc.full}} {
			if got := f.Date(tm, c.style); got != c.want {
				t.Errorf("%s Date %v = %q, want %q", tc.loc, c.style, got, c.want)
			}
		}
	}
}

func TestTimes(t *testing.T) {
	tm := time.Date(2026, 1, 5, 15, 4, 5, 0, time.UTC)
	cet := time.Date(2026, 1, 5, 0, 30, 0, 0, time.FixedZone("CET", 3600))
	tests := []struct {
		loc   string
		t     time.Time
		style DateStyle
		want  string
	}{
		{"en", tm, Short, "3:04 PM"},
		{"en", tm, Medium, "3:04:05 PM"},
		{"en", tm, Long, "3:04:05 PM UTC"},
		{"en", cet, Short, "12:30 AM"},
		{"es", tm, Short, "15:04"},
		{"de", tm, Medium, "15:04:05"},
		{"ja", tm, Full, "15時04分05秒 UTC"},
		{"ar", tm, Short, "٣:٠٤ م"},
		{"en", cet, Long, "12:30:00 AM CET"},
		{"en", time.Date(2026, 1, 5, 12, 0, 0, 0, time.FixedZone("", -5*3600-30*60)), Long, "12:00:00 PM GMT-5:30"},
	}
	for _, tc := range tests {
		if got := For(tc.loc).Time(tc.t, tc.style); got != tc.want {
			t.Errorf("%s Time %v = %q, want %q", tc.loc, tc.style, got, tc.want)
		}
	}
	if got := For("en").DateTime(tm, Medium, Short); got != "Jan 5, 2026, 3:04 PM" {
		t.Errorf("DateTime = %q", got)
	}
	if got := For("en").DateTime(tm, Long, Short); got != "January 5, 2026 at 3:04 PM" {
		t.Errorf("DateTime long = %q", got)
	}
	if got := For("es").DateTime(tm, Long, Short); got != "5 de enero de 2026 a las 15:04" {
		t.Errorf("DateTime es = %q", got)
	}
}

func TestDateBC(t *testing.T) {
	// Year 0 is 1 BC; the era appears in Thai's Gregorian patterns.
	bc := time.Date(0, 3, 1, 0, 0, 0, 0, time.UTC)
	if got := For("th").Date(bc, Long); got != "1 มีนาคม ค.ศ. 1" && got != "1 มีนาคม ก่อน ค.ศ. 1" {
		t.Errorf("th BC = %q", got)
	}
}

// A locale without its own data uses the closest one.
func TestDateLocaleResolution(t *testing.T) {
	tm := time.Date(2026, 1, 5, 15, 4, 5, 0, time.UTC)
	tests := []struct{ loc, want string }{
		{"es-PE", "5 de enero de 2026"},   // es-419
		{"pt-AO", "5 de janeiro de 2026"}, // pt-PT or pt
		{"zh-Hans-CN", "2026年1月5日"},       // zh
		{"zh-TW", "2026年1月5日"},            // zh-Hant
		{"en-NZ", "5 January 2026"},       // own data
		{"en-SG", "5 January 2026"},       // en-001 (world English)
		{"sr", "5. јануар 2026."},         // Cyrillic
		{"sr-Latn-ME", "5. januar 2026."}, // Latin
		{"ti", "January 5, 2026"},         // no data: English
		{"nb-NO", "5. januar 2026"},       // nb
		{"no", "5. januar 2026"},          // no
	}
	for _, tc := range tests {
		if got := For(tc.loc).Date(tm, Long); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.loc, got, tc.want)
		}
	}
}

func TestLists(t *testing.T) {
	abc := []string{"a", "b", "c"}
	abcd := []string{"a", "b", "c", "d"}
	tests := []struct {
		loc  string
		in   []string
		want string
	}{
		{"en", nil, ""},
		{"en", []string{"a"}, "a"},
		{"en", []string{"a", "b"}, "a and b"},
		{"en", abc, "a, b, and c"},
		{"en-GB", abc, "a, b and c"},
		{"en", abcd, "a, b, c, and d"},
		{"es", abc, "a, b y c"},
		{"es", []string{"a", "b"}, "a y b"},
		{"fr", abc, "a, b et c"},
		{"de", abc, "a, b und c"},
		{"ja", abc, "a、b、c"},
		{"ja", abcd, "a、b、c、d"},
		{"zh", abc, "a、b和c"},
		{"ar", abc, "a وb وc"},
		{"ru", abc, "a, b и c"},
	}
	for _, tc := range tests {
		if got := For(tc.loc).List(tc.in); got != tc.want {
			t.Errorf("%s %q = %q, want %q", tc.loc, tc.in, got, tc.want)
		}
	}
}

func TestDir(t *testing.T) {
	for tag, want := range map[string]Direction{
		"en": LTR, "es-MX": LTR, "ja": LTR, "zh-Hant": LTR, "sr-Latn": LTR, "ku": LTR,
		"ar": RTL, "ar-EG": RTL, "he": RTL, "fa": RTL, "ur": RTL, "ps": RTL, "sd": RTL,
		"yi": RTL, "ckb": RTL, "ku-Arab": RTL, "dv": RTL, "az-Arab": RTL, "ar_SA": RTL,
		"": LTR, "not a tag!": LTR,
	} {
		if got := Dir(tag); got != want {
			t.Errorf("Dir(%q) = %v, want %v", tag, got, want)
		}
	}
	if For("ar").Dir() != RTL || For("en").Dir() != LTR {
		t.Error("Formatter.Dir")
	}
	if RTL.String() != "RTL" || LTR.String() != "LTR" {
		t.Error("Direction.String")
	}
}

func TestForInvalidTag(t *testing.T) {
	defer func() {
		r := recover()
		s, _ := r.(string)
		if s == "" || !strings.Contains(s, `For("not a tag!")`) || !strings.Contains(s, "BCP 47") {
			t.Errorf("panic = %v", r)
		}
	}()
	For("not a tag!")
}

func TestUnderscoreTags(t *testing.T) {
	if got := For("es_MX").Locale(); got != "es-MX" {
		t.Errorf("Locale = %q", got)
	}
}

func TestInvalidDateStyle(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("no panic for DateStyle(9)")
		}
	}()
	For("en").Date(time.Now(), DateStyle(9))
}
