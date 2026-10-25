package main

import (
	"fmt"
	"maps"
	"strings"

	"github.com/kuronosu/tela/kit"
)

// styles are the two looks of the gallery, by the name the Style picker
// uses. "default" is nil, which kit.StyleProvider reads as kit's own recipes;
// "rounded" is roundedStyle. A style is built once and never edited after it
// is given to a provider.
var styles = map[string]*kit.Style{
	"default": nil,
	"rounded": roundedStyle(),
}

// roundedStyle is the customization the Style picker shows: a second look
// for the whole kit, made of kit.DefaultStyle and a few edits of its class
// strings, without a copy of any component's source. Pill-shaped controls,
// large corners and a violet accent in place of the theme's neutral one.
//
// A recipe is plain data: each edit is a Replace of one class on the string
// that has it (swap) or a new entry in a Variants map. swap panics when the
// class is not there, so an edit does not silently stop applying when the
// default recipes change.
func roundedStyle() *kit.Style {
	s := kit.DefaultStyle()

	// Buttons: pills, with a violet primary. The variants map takes a new
	// look as easily as a changed one.
	swap(&s.Button.Root, "rounded-md", "rounded-full")
	swapAll(s.Button.Sizes, "rounded-md", "rounded-full")
	s.Button.Variants[kit.Default] = "bg-violet-600 text-white shadow-md shadow-violet-500/25 hover:bg-violet-600/90 dark:bg-violet-500 dark:hover:bg-violet-500/90"
	s.Button.Variants[kit.Outline] += " hover:border-violet-400 dark:hover:border-violet-400"
	s.Button.Variants[kit.Link] = "text-violet-600 underline-offset-4 hover:underline dark:text-violet-400"

	swapIn(s.Badge.Variants, kit.Default, "bg-primary text-primary-foreground", "bg-violet-600 text-white dark:bg-violet-500")

	// Cards and dialogs: large corners and a softer shadow.
	swap(&s.Card.Root, "rounded-xl", "rounded-3xl")
	swap(&s.Card.Root, "shadow-sm", "shadow-lg shadow-violet-500/5")
	swap(&s.Dialog.Root, "rounded-lg", "rounded-3xl")
	swap(&s.AlertDialog.Root, "rounded-lg", "rounded-3xl")
	swap(&s.Popover.Root, "rounded-md", "rounded-2xl")
	swap(&s.DropdownMenu.Content, "rounded-md", "rounded-2xl")
	swap(&s.DropdownMenu.Item, "rounded-sm", "rounded-xl")
	swap(&s.ContextMenu.Content, "rounded-md", "rounded-2xl")
	swap(&s.ContextMenu.Item, "rounded-sm", "rounded-xl")

	// Fields: pills with a little more room.
	swap(&s.Input.Root, "rounded-md", "rounded-full")
	swap(&s.Input.Root, "px-3", "px-4")
	swap(&s.Textarea.Root, "rounded-md", "rounded-2xl")
	swap(&s.Select.Root, "rounded-md", "rounded-full")
	swap(&s.Select.Root, "px-3", "px-4")
	swap(&s.Select.Content, "rounded-md", "rounded-2xl")
	swap(&s.Select.Item, "rounded-sm", "rounded-xl")

	// Choice controls: round checkboxes and violet when on.
	swap(&s.Checkbox.Root, "rounded-[4px]", "rounded-full")
	swap(&s.Checkbox.Root, "checked:border-primary checked:bg-primary", "checked:border-violet-600 checked:bg-violet-600")
	swap(&s.Checkbox.Root, "dark:checked:bg-primary", "dark:checked:border-violet-500 dark:checked:bg-violet-500")
	swap(&s.Switch.Root, "checked:bg-primary", "checked:bg-violet-600 dark:checked:bg-violet-500")
	swap(&s.Slider.Range, "bg-primary", "bg-violet-600 dark:bg-violet-500")
	swap(&s.Slider.Thumb, "border-primary", "border-violet-600 dark:border-violet-500")
	swap(&s.Progress.Root, "bg-primary/20", "bg-violet-500/20")
	swap(&s.Progress.Indicator, "bg-primary", "bg-linear-to-r from-violet-500 to-fuchsia-500")

	// Segmented controls and tabs: pills, with a tinted pressed state.
	swap(&s.Toggle.Root, "rounded-md", "rounded-full")
	s.Toggle.On = "aria-pressed:bg-violet-100 aria-pressed:text-violet-700 checked:bg-violet-100 checked:text-violet-700 " +
		"dark:aria-pressed:bg-violet-500/25 dark:aria-pressed:text-violet-200 dark:checked:bg-violet-500/25 dark:checked:text-violet-200"
	swap(&s.ToggleGroup.Root, "rounded-md", "rounded-full")
	swap(&s.ToggleGroup.ItemWithoutSpacing, "first:rounded-s-md last:rounded-e-md", "first:rounded-s-full last:rounded-e-full")
	swap(&s.Tabs.List, "rounded-lg", "rounded-full")
	swap(&s.Tabs.Trigger, "rounded-md", "rounded-full")

	// The calendar's selected day.
	swap(&s.Calendar.DayButtonWithSelected, "bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground dark:hover:bg-primary",
		"bg-violet-600 text-white hover:bg-violet-600 hover:text-white dark:bg-violet-500 dark:hover:bg-violet-500")

	return &s
}

// swap replaces every occurrence of the class old in a recipe's string with
// repl, and panics when there is none: the edit would be a silent no-op.
func swap(recipe *string, old, repl string) {
	if !strings.Contains(*recipe, old) {
		panic(fmt.Sprintf("examples/kit: the recipe %q has no class %q to replace; kit's default style changed", *recipe, old))
	}
	*recipe = strings.ReplaceAll(*recipe, old, repl)
}

// swapIn does it for one entry of a Variants or Sizes map.
func swapIn[K comparable](recipes map[K]string, key K, old, repl string) {
	v := recipes[key]
	swap(&v, old, repl)
	recipes[key] = v
}

// swapAll does the same for the entries of a Variants or Sizes map that
// have the class.
func swapAll[K comparable](recipes map[K]string, old, repl string) {
	for k, v := range maps.All(recipes) {
		if strings.Contains(v, old) {
			recipes[k] = strings.ReplaceAll(v, old, repl)
		}
	}
}
