package main

import (
	"fmt"
	"math"
	"strings"
)

// Period is one of the three ranges the header's segmented control picks.
// Every number on the page comes from the Period at the selected index.
type Period struct {
	Label   string
	Revenue float32 // dollars
	Subs    float32
	Orders  float32
	Goal    float32 // percent of the monthly goal
	Caption string  // what a bar of the chart covers
	XLabels [bars]string
}

var periods = []Period{
	{"7D", 12480.50, 342, 1204, 41, "7 days, per 14 hours",
		[bars]string{"Mon", "", "Tue", "", "Wed", "", "Thu", "", "Fri", "", "Sat", "Sun"}},
	{"30D", 45231.89, 2350, 12234, 72, "30 days, per 2.5 days",
		[bars]string{"1", "3", "5", "8", "10", "13", "15", "18", "20", "23", "25", "28"}},
	{"90D", 128904.12, 6718, 38902, 94, "90 days, per week",
		[bars]string{"W1", "W2", "W3", "W4", "W5", "W6", "W7", "W8", "W9", "W10", "W11", "W12"}},
}

// Metric is one tab of the revenue chart.
type Metric struct {
	ID, Label string
	Unit      string  // prefix of the tooltip's value
	Scale     float32 // value of a full-height bar
	seed      float64
}

var metrics = []Metric{
	{"revenue", "Revenue", "$", 9000, 0.4},
	{"orders", "Orders", "", 2400, 1.9},
	{"visitors", "Visitors", "", 18000, 3.1},
}

const bars = 12

// series is the 12 bar heights (0 to 1) of a metric in a period. It is a
// pair of sines, so the data is deterministic, looks organic and differs
// between metrics and ranges.
func series(m Metric, period int) [bars]float32 {
	var out [bars]float32
	for i := range out {
		x := float64(i)
		v := 0.52 + 0.26*math.Sin(x*0.85+m.seed+float64(period)*0.9) + 0.14*math.Sin(x*2.3+m.seed*1.7)
		out[i] = float32(max(0.12, min(0.98, v)))
	}
	return out
}

// Source is a slice of the traffic donut.
type Source struct {
	Name  string
	Pct   int
	Color string // a Tailwind color name, for the legend dot
	Hex   string // the same color for the conic gradient
}

var sources = []Source{
	{"Direct", 42, "bg-indigo-500", "#6366f1"},
	{"Search", 27, "bg-fuchsia-500", "#d946ef"},
	{"Social", 19, "bg-sky-500", "#0ea5e9"},
	{"Referral", 12, "bg-amber-400", "#fbbf24"},
}

// donut is the conic-gradient class of sources: each color holds from the
// end of the previous slice to its own end.
func donut() string {
	var stops []string
	at := 0
	for _, s := range sources {
		stops = append(stops, fmt.Sprintf("%s_%d%%_%d%%", s.Hex, at, at+s.Pct))
		at += s.Pct
	}
	return "bg-[conic-gradient(" + strings.Join(stops, ",") + ")]"
}

// Task is a row of the priorities list.
type Task struct {
	ID, Title, Tag string
	Dot            string // dot color class
	Priority, Due  int    // smaller is more urgent, sooner
	DueLabel       string
}

var tasks = []Task{
	{"export", "Ship the CSV export", "Product", "bg-fuchsia-500", 2, 3, "Thu"},
	{"billing", "Fix the failed invoice retries", "Billing", "bg-rose-500", 1, 1, "Today"},
	{"onboarding", "Review the onboarding emails", "Growth", "bg-sky-500", 4, 2, "Wed"},
	{"churn", "Interview three churned customers", "Research", "bg-amber-500", 3, 5, "Mon"},
	{"docs", "Publish the API changelog", "Docs", "bg-emerald-500", 5, 4, "Fri"},
}

// Sale is a row of the activity list.
type Sale struct {
	Name, Item, Amount, Initials, Tint string
}

var sales = []Sale{
	{"Olivia Martin", "Pro plan, yearly", "+$1,999.00", "OM", "from-rose-400 to-orange-400"},
	{"Jackson Lee", "Add-on: seats x5", "+$39.00", "JL", "from-sky-400 to-indigo-500"},
	{"Isabella Nguyen", "Pro plan, monthly", "+$299.00", "IN", "from-amber-400 to-pink-500"},
	{"William Kim", "Team workspace", "+$99.00", "WK", "from-emerald-400 to-cyan-500"},
}

// commas formats n with thousands separators.
func commas(n int64) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// dollars formats v as $12,345.67.
func dollars(v float32) string {
	cents := int64(math.Round(float64(v) * 100))
	return fmt.Sprintf("$%s.%02d", commas(cents/100), cents%100)
}

// count formats v as a whole number with separators.
func count(v float32) string { return commas(int64(math.Round(float64(v)))) }
