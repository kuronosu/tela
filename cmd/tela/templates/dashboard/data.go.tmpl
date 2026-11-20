package main

import (
	"fmt"
	"math"
)

// Period is one of the three ranges of the period switch. Every number of
// the overview comes from the Period at the selected index.
type Period struct {
	Label    string
	Revenue  float32 // dollars
	Subs     float32
	Orders   float32
	Visitors float32
	Caption  string // what the spectrum covers
	XLabels  [12]string
}

var periods = []Period{
	{"7D", 12480.50, 342, 1204, 17450, "7 days",
		[12]string{"Mon", "", "Tue", "", "Wed", "", "Thu", "", "Fri", "", "Sat", "Sun"}},
	{"30D", 45231.89, 2350, 12234, 169917, "30 days",
		[12]string{"1", "3", "5", "8", "10", "13", "15", "18", "20", "23", "25", "28"}},
	{"90D", 128904.12, 6718, 38902, 525700, "90 days",
		[12]string{"W1", "W2", "W3", "W4", "W5", "W6", "W7", "W8", "W9", "W10", "W11", "W12"}},
}

// Metric is a tab of the headline: the number it shows, the scale of its
// spectrum and its change, by period.
type Metric struct {
	ID, Label string
	Value     func(Period) float32
	Format    func(float32) string
	Scale     float32 // the value of a full-height bar of the spectrum
	Seed      float64 // shapes its series
	Deltas    [3]string
}

var metrics = []Metric{
	{"revenue", "Revenue", func(p Period) float32 { return p.Revenue }, dollars, 9000, 2,
		[3]string{"8.2%", "20.1%", "31.4%"}},
	{"orders", "Orders", func(p Period) float32 { return p.Orders }, count, 2400, 0.7,
		[3]string{"3.1%", "19.0%", "12.6%"}},
	{"visitors", "Visitors", func(p Period) float32 { return p.Visitors }, count, 9000, 4.2,
		[3]string{"5.5%", "11.8%", "9.2%"}},
}

// metricOf returns the metric with the ID.
func metricOf(id string) Metric {
	for _, m := range metrics {
		if m.ID == id {
			return m
		}
	}
	return metrics[0]
}

// Stat is one of the four figures under the headline.
type Stat struct {
	Label  string
	Value  func(Period) float32
	Format func(float32) string
	Deltas [3]string
	Up     [3]bool
}

var stats = []Stat{
	{"Subscribers", func(p Period) float32 { return p.Subs }, count,
		[3]string{"+2.4%", "+18.0%", "-1.3%"}, [3]bool{true, true, false}},
	{"Orders", func(p Period) float32 { return p.Orders }, count,
		[3]string{"-3.1%", "+19.0%", "+12.6%"}, [3]bool{false, true, true}},
	{"Conversion", func(p Period) float32 { return p.Orders / p.Visitors * 100 },
		func(v float32) string { return fmt.Sprintf("%.1f%%", v) },
		[3]string{"+0.4%", "-1.3%", "+0.9%"}, [3]bool{true, false, true}},
	{"Avg. order", func(p Period) float32 { return p.Revenue / p.Orders * 10 },
		func(v float32) string { return fmt.Sprintf("$%.2f", v) },
		[3]string{"+1.2%", "+4.4%", "+2.0%"}, [3]bool{true, true, true}},
}

// series is n bar heights from 0.1 to 0.98: a sum of sines, so the data is
// deterministic, looks organic and differs between metrics and periods.
func series(n int, seed float64, period int) []float32 {
	out := make([]float32, n)
	k := 12 / float64(n)
	for i := range out {
		x := float64(i) * k
		v := 0.52 + 0.26*math.Sin(x*0.85+seed+float64(period)*0.9) + 0.14*math.Sin(x*2.3+seed*1.7) + 0.05*math.Sin(float64(i)*1.9+seed)
		out[i] = float32(max(0.1, min(0.98, v)))
	}
	return out
}

// Source is a line of the traffic meters.
type Source struct {
	Name string
	Pct  int
}

var sources = []Source{{"Direct", 42}, {"Search", 27}, {"Social", 19}, {"Referral", 12}}

// Event is a line of the activity feed.
type Event struct{ Who, What, Ago, Initials string }

var feed = []Event{
	{"Olivia Martin", "upgraded to Pro, yearly", "2m", "OM"},
	{"Jackson Lee", "added 5 seats", "14m", "JL"},
	{"Isabella Nguyen", "invoice #4881 paid", "41m", "IN"},
	{"William Kim", "opened a support thread", "1h", "WK"},
	{"Sofia Garcia", "cancelled Team plan", "3h", "SG"},
}

// Order is a row of the orders tables.
type Order struct {
	ID       string
	Customer string
	Email    string
	Plan     string
	Status   string // Paid, Pending, Refunded or Failed
	Amount   float32
	Date     string
	Initials string
}

// statuses are the filters of the orders view, "All" first.
var statuses = []string{"All", "Paid", "Pending", "Refunded", "Failed"}

// orders are 240 deterministic rows, newest first.
var orders = makeOrders(240)

func makeOrders(n int) []Order {
	first := []string{"Olivia", "Jackson", "Isabella", "William", "Sofia", "Liam", "Mei", "Ana", "Noah", "Emma", "Lucas", "Mia", "Ethan", "Ava", "Diego", "Yuki", "Hana", "Omar", "Priya", "Tomas"}
	last := []string{"Martin", "Lee", "Nguyen", "Kim", "Garcia", "Tanaka", "Silva", "Brown", "Rossi", "Khan", "Novak", "Dubois", "Sato", "Costa", "Weber", "Okafor"}
	plans := []string{"Pro", "Team", "Starter", "Enterprise", "Pro"}
	price := map[string]float32{"Starter": 29, "Pro": 99, "Team": 299, "Enterprise": 1499}
	status := []string{"Paid", "Paid", "Paid", "Pending", "Paid", "Refunded", "Paid", "Failed", "Paid", "Pending"}
	months := []string{"Oct", "Oct", "Oct", "Sep", "Sep", "Sep", "Aug", "Aug"}
	out := make([]Order, n)
	for i := range out {
		f, l := first[(i*7+3)%len(first)], last[(i*5+1)%len(last)]
		plan := plans[(i*3)%len(plans)]
		amount := price[plan] * float32(1+(i*13)%4)
		if i%11 == 0 {
			amount += 0.5 * float32(i%7)
		}
		out[i] = Order{
			ID:       fmt.Sprintf("#%d", 4891-i),
			Customer: f + " " + l,
			Email:    fmt.Sprintf("%s.%s@example.com", lower(f), lower(l)),
			Plan:     plan,
			Status:   status[(i*7+2)%len(status)],
			Amount:   amount,
			Date:     fmt.Sprintf("%s %d", months[(i/30)%len(months)], 30-(i%28)),
			Initials: f[:1] + l[:1],
		}
	}
	return out
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// ordersWith returns the orders with the status, or all of them.
func ordersWith(status string) []Order {
	if status == "All" || status == "" {
		return orders
	}
	var out []Order
	for _, o := range orders {
		if o.Status == status {
			out = append(out, o)
		}
	}
	return out
}

// commas formats n with thousands separators.
func commas(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + commas(-n)
	}
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

// upto returns 0, 1, ... n-1: the keys of a list keyed by position.
func upto(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
