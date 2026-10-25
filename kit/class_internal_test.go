package kit

import "testing"

// joinClass puts the classes with a variant after the plain ones, keeping
// the order within each group, so a recipe gets Tailwind's stylesheet
// precedence under Tela's last-wins class order.
func TestJoinClassOrder(t *testing.T) {
	for _, c := range []struct {
		parts []string
		want  string
	}{
		{[]string{"focus-visible:border-ring outline-none", "border border-border"}, "outline-none border border-border focus-visible:border-ring"},
		{[]string{"hover:bg-accent bg-background", "", "  ", "dark:bg-input/30 px-2"}, "bg-background px-2 hover:bg-accent dark:bg-input/30"},
		{[]string{"bg-[url(a:b.png)] [clip-path:circle(50%)] md:hover:underline"}, "bg-[url(a:b.png)] [clip-path:circle(50%)] md:hover:underline"},
		{[]string{"aria-[sort=ascending]:rotate-180 size-4"}, "size-4 aria-[sort=ascending]:rotate-180"},
		{nil, ""},
	} {
		if got := joinClass(c.parts...); got != c.want {
			t.Errorf("joinClass(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
}
