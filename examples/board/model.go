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

// Task is the data of a card; the card with ID n is tasks[n].
type Task struct {
	Title    string
	Note     string
	Tag      string
	Colors   string // Tailwind classes of the tag chip
	Assignee string // initials
	Comments int
}

var columnNames = [3]string{"To do", "In progress", "Done"}

// dots are the colors of the dots before the column names.
var dots = [3]string{"bg-zinc-400", "bg-amber-500", "bg-emerald-500"}

const (
	tagDesign   = "bg-violet-100 text-violet-700 dark:bg-violet-500/20 dark:text-violet-300"
	tagDocs     = "bg-sky-100 text-sky-700 dark:bg-sky-500/20 dark:text-sky-300"
	tagA11y     = "bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300"
	tagFrontend = "bg-indigo-100 text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300"
	tagBug      = "bg-rose-100 text-rose-700 dark:bg-rose-500/20 dark:text-rose-300"
	tagDevOps   = "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300"
)

// tasks is indexed by card ID; tasks[0] is unused.
var tasks = [...]Task{
	0:  {},
	1:  {"Design the onboarding flow", "Three screens, one goal: first project in under a minute.", "Design", tagDesign, "AL", 3},
	2:  {"Write the API reference", "Every exported function gets an example.", "Docs", tagDocs, "MK", 0},
	3:  {"Audit color contrast", "Check text on tinted surfaces against WCAG AA.", "A11y", tagA11y, "JS", 1},
	4:  {"Implement dark mode", "Pair every color with its dark: variant.", "Frontend", tagFrontend, "OM", 5},
	5:  {"Fix the login redirect bug", "Users land on a blank page after signing in.", "Bug", tagBug, "WK", 2},
	6:  {"Set up the CI pipeline", "Build, vet and test on three platforms.", "DevOps", tagDevOps, "AL", 0},
	7:  {"Ship the v0.4 changelog", "Group entries by user-visible change.", "Docs", tagDocs, "MK", 4},
	8:  {"Define the color tokens", "Neutral scale plus one accent.", "Design", tagDesign, "JS", 2},
	9:  {"Migrate the settings page", "Move preferences to the new form components.", "Frontend", tagFrontend, "OM", 1},
	10: {"Review the pricing copy", "Make the plan names consistent everywhere.", "Docs", tagDocs, "WK", 0},
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
