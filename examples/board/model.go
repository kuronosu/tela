package main

// maxCards is how many cards a column can hold. Columns are fixed-size
// arrays of card IDs so that the whole Board is a comparable value: it can
// be compared with == and passed in the props of components that skip
// their build when it does not change.
const maxCards = 8

// Column is the cards of one column, top to bottom. Card IDs start at 1;
// 0 marks a free slot.
type Column [maxCards]int

// Board is the three columns.
type Board [3]Column

// Task is the data of a card.
type Task struct {
	Title    string
	Note     string
	Tag      string
	Assignee string // initials
	Comments int
}

// Tasks is the data of every card, indexed by card ID (Tasks[0] is unused).
// It is an array, so it is comparable and sits in UseState and in the
// props of components that skip their build when it does not change.
type Tasks [11]Task

var columnNames = [3]string{"To do", "In progress", "Done"}

// dots are the colors of the dots before the column names: decoration,
// the same in both schemes.
var dots = [3]string{"bg-zinc-400", "bg-amber-500", "bg-emerald-500"}

// tags are the labels a card can have, and tagDots the color of the dot
// that goes before each (decoration as well).
var tags = []string{"Design", "Docs", "A11y", "Frontend", "Bug", "DevOps"}

var tagDots = map[string]string{
	"Design":   "bg-violet-500",
	"Docs":     "bg-sky-500",
	"A11y":     "bg-amber-500",
	"Frontend": "bg-indigo-500",
	"Bug":      "bg-rose-500",
	"DevOps":   "bg-emerald-500",
}

// tasks are the cards at the start.
var tasks = Tasks{
	1:  {"Design the onboarding flow", "Three screens, one goal: first project in under a minute.", "Design", "AL", 3},
	2:  {"Write the API reference", "Every exported function gets an example.", "Docs", "MK", 0},
	3:  {"Audit color contrast", "Check text on tinted surfaces against WCAG AA.", "A11y", "JS", 1},
	4:  {"Implement dark mode", "Use theme tokens instead of pairs of colors.", "Frontend", "OM", 5},
	5:  {"Fix the login redirect bug", "Users land on a blank page after signing in.", "Bug", "WK", 2},
	6:  {"Set up the CI pipeline", "Build, vet and test on three platforms.", "DevOps", "AL", 0},
	7:  {"Ship the v0.4 changelog", "Group entries by user-visible change.", "Docs", "MK", 4},
	8:  {"Define the color tokens", "Neutral scale plus one accent.", "Design", "JS", 2},
	9:  {"Migrate the settings page", "Move preferences to the new form components.", "Frontend", "OM", 1},
	10: {"Review the pricing copy", "Make the plan names consistent everywhere.", "Docs", "WK", 0},
}

// Initial is the board at the start.
var Initial = Board{
	{1, 2, 3, 9},
	{4, 5, 10},
	{6, 7, 8},
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
// would go before (0 means at the end of the column). The zero Target is
// "nowhere": Valid is false.
type Target struct {
	Col    int
	Before int
	Valid  bool
}

// Move returns b with card id moved to the target: removed from its
// column and inserted before t.Before in column t.Col (or at its end).
// A full target column, or an invalid target, leaves b unchanged.
func (b Board) Move(id int, t Target) Board {
	if !t.Valid || id == 0 || t.Before == id {
		return b
	}
	from := -1
	for ci, col := range b {
		for _, c := range col {
			if c == id {
				from = ci
			}
		}
	}
	if from < 0 {
		return b
	}
	if from != t.Col && b[t.Col].Len() >= maxCards {
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
	dst = append(dst[:at], append([]int{id}, dst[at:]...)...)
	ids[t.Col] = dst
	var out Board
	for ci := range ids {
		copy(out[ci][:], ids[ci])
	}
	return out
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

// Full reports which columns cannot take another card.
func (b Board) Full() [3]bool {
	var full [3]bool
	for ci, col := range b {
		full[ci] = col.Len() >= maxCards
	}
	return full
}
