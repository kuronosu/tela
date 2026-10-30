package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/kit"
)

// The tasks are one model shown two ways: the board (three columns of
// cards) and the table (every task, finished ones last). A task is
// finished when its card is in the Done column, so checking it off in the
// table moves its card, and dragging a card to Done checks it.

// maxCards is how many cards a column holds. Columns are fixed-size arrays
// of task IDs so that the Board is a comparable value: it sits in UseState
// and in the props of components that skip their build when it does not
// change.
const maxCards = 8

// Column is the cards of one column, top to bottom. Task IDs start at 1;
// 0 marks a free slot.
type Column [maxCards]int

// Board is the three columns.
type Board [3]Column

// Task is a card of the board and a row of the table.
type Task struct {
	Key      string // TEL-142
	Title    string
	Note     string
	Tag      string
	Assignee string // initials
	Comments int
	Priority int // 1 urgent ... 4 low
	Due      string
	DueIn    int // days from today, to sort by
}

// Tasks is the data of every task by ID (Tasks[0] is unused): an array, so
// it is comparable too. New tasks take the free IDs.
type Tasks [25]Task

const (
	todo = iota
	doing
	done
)

var columnNames = [3]string{"To do", "In progress", "Done"}

// tags are the teams a task can belong to.
var tags = []string{"A11y", "Billing", "Design", "Docs", "Growth", "Platform", "Product", "Research"}

var priorityNames = [5]string{"", "Urgent", "High", "Medium", "Low"}

var initialTasks = Tasks{
	1:  {"TEL-101", "Fix failed invoice retries", "Retry with backoff, then email the owner.", "Billing", "AL", 4, 1, "Today", 0},
	2:  {"TEL-104", "Ship the CSV export", "Streams, no memory spike on big workspaces.", "Product", "MK", 2, 2, "Thu", 3},
	3:  {"TEL-108", "Review the onboarding emails", "Three emails, one call to action each.", "Growth", "JS", 0, 3, "Wed", 2},
	4:  {"TEL-112", "Interview three churned customers", "Why did annual plans lapse in September?", "Research", "OM", 1, 3, "Mon", 5},
	5:  {"TEL-115", "Publish the API changelog", "Group by what a developer has to change.", "Docs", "WK", 0, 4, "Fri", 4},
	6:  {"TEL-117", "Dark mode for the invoice PDF", "Match the app, keep it printable.", "Design", "AL", 3, 3, "Nov 4", 6},
	8:  {"TEL-123", "Audit color contrast", "Text on tinted surfaces against WCAG AA.", "A11y", "JS", 1, 2, "Thu", 3},
	9:  {"TEL-126", "Migrate settings to forms", "Move preferences to the new components.", "Product", "OM", 2, 4, "Nov 6", 8},
	10: {"TEL-129", "Set up the staging pipeline", "Build, vet and test on three platforms.", "Platform", "WK", 0, 3, "Oct 24", -6},
	11: {"TEL-131", "Write the Q3 revenue summary", "Numbers by plan, with the churn note.", "Research", "AL", 6, 2, "Oct 27", -3},
	12: {"TEL-133", "Tidy the pricing page copy", "Plan names consistent everywhere.", "Docs", "MK", 0, 4, "Oct 28", -2},
	13: {"TEL-136", "Webhook signature checks", "Reject replays older than five minutes.", "Platform", "JS", 2, 1, "Oct 29", -1},
}

var initialBoard = Board{
	{1, 2, 3, 4},
	{8, 6, 9, 5},
	{10, 11, 12, 13},
}

// Len returns the number of cards of the column.
func (c Column) Len() int {
	n := 0
	for _, id := range c {
		if id != 0 {
			n++
		}
	}
	return n
}

// IDs returns the cards of the column as a slice.
func (c Column) IDs() []int { return append([]int(nil), c[:c.Len()]...) }

// Target is where a dragged card would land: a column and the card it
// would go before (0: at the end). The zero Target is "nowhere".
type Target struct {
	Col    int
	Before int
	Valid  bool
}

// Move returns b with card id moved to the target: removed from its column
// and inserted before t.Before in column t.Col, or at its end. A full
// target column, or an invalid target, leaves b unchanged.
func (b Board) Move(id int, t Target) Board {
	if !t.Valid || id == 0 || t.Before == id {
		return b
	}
	from := b.ColumnOf(id)
	if from < 0 || (from != t.Col && b[t.Col].Len() >= maxCards) {
		return b
	}
	var ids [3][]int
	for ci, col := range b {
		for _, c := range col.IDs() {
			if c != id {
				ids[ci] = append(ids[ci], c)
			}
		}
	}
	dst := ids[t.Col]
	at := len(dst)
	for i, c := range dst {
		if c == t.Before {
			at = i
		}
	}
	ids[t.Col] = slices.Insert(dst, at, id)
	var out Board
	for ci := range ids {
		copy(out[ci][:], ids[ci])
	}
	return out
}

// Add returns b with card id at the end of column col (unchanged if full).
func (b Board) Add(id, col int) Board {
	if n := b[col].Len(); n < maxCards {
		b[col][n] = id
	}
	return b
}

// Remove returns b without card id.
func (b Board) Remove(id int) Board {
	var out Board
	for ci, col := range b {
		n := 0
		for _, c := range col.IDs() {
			if c != id {
				out[ci][n] = c
				n++
			}
		}
	}
	return out
}

// ColumnOf returns the column that holds card id, or -1.
func (b Board) ColumnOf(id int) int {
	for ci, col := range b {
		if slices.Contains(col[:], id) {
			return ci
		}
	}
	return -1
}

// Full reports which columns cannot take another card.
func (b Board) Full() [3]bool {
	var full [3]bool
	for ci, col := range b {
		full[ci] = col.Len() >= maxCards
	}
	return full
}

// Count is the number of cards on the board.
func (b Board) Count() int { return b[0].Len() + b[1].Len() + b[2].Len() }

// freeID returns an unused task ID, or 0 when every slot is taken.
func (ts Tasks) freeID() int {
	for id := 1; id < len(ts); id++ {
		if ts[id].Key == "" {
			return id
		}
	}
	return 0
}

// nextKey is the key of a new task: one past the highest.
func (ts Tasks) nextKey() string {
	high := 0
	for _, t := range ts {
		var n int
		if _, err := fmt.Sscanf(t.Key, "TEL-%d", &n); err == nil && n > high {
			high = n
		}
	}
	return "TEL-" + strconv.Itoa(high+1)
}

// Row is a row of the task table.
type Row struct {
	ID   int
	Task Task
	Done bool
}

// rows returns the tasks of the board in the order of the sort, finished
// ones last. The table shows them in this order (its columns are Sortable,
// which leaves the order to the page), so that a finished task sinks
// whatever the sort.
func rows(b Board, ts Tasks, sort kit.SortState) []Row {
	var out []Row
	for ci, col := range b {
		for _, id := range col.IDs() {
			out = append(out, Row{ID: id, Task: ts[id], Done: ci == done})
		}
	}
	by := func(a, c Row) int { return a.Task.Priority - c.Task.Priority }
	switch sort.Column {
	case "title":
		by = func(a, c Row) int { return strings.Compare(a.Task.Title, c.Task.Title) }
	case "due":
		by = func(a, c Row) int { return a.Task.DueIn - c.Task.DueIn }
	case "tag":
		by = func(a, c Row) int { return strings.Compare(a.Task.Tag, c.Task.Tag) }
	}
	slices.SortStableFunc(out, func(a, c Row) int {
		if a.Done != c.Done {
			if a.Done {
				return 1
			}
			return -1
		}
		r := by(a, c)
		if sort.Direction == kit.Descending {
			r = -r
		}
		if r == 0 {
			return strings.Compare(a.Task.Key, c.Task.Key)
		}
		return r
	})
	return out
}
