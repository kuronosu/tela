package main

import "github.com/kuronosu/tela"

// pages are the components of the kit, in the order of the sidebar
// (alphabetical). The preview of a page is named after Name.
var pages = []page{
	{"AlertDialog", "Alert Dialog", "kit.AlertDialog", "A modal question that needs an answer: it is not closed by a press outside, and its focus starts on Cancel.", tela.C(alertDialogPage)},
	{"Avatar", "Avatar", "kit.Avatar", "A round picture of a person, with their initials while there is no picture.", tela.C(avatarPage)},
	{"Badge", "Badge", "kit.Badge", "A small label for a status, a count or a tag.", tela.C(badgePage)},
	{"Button", "Button", "kit.Button", "A button in six variants and eight sizes, with icon slots. It is the trigger of every overlay.", tela.C(buttonPage)},
	{"Calendar", "Calendar", "kit.Calendar", "A month grid that selects one date, with limits, several months and the names, digits and first weekday of the locale.", tela.C(calendarPage)},
	{"Card", "Card", "kit.Card", "A surface with a header, content and a footer, each a slot.", tela.C(cardPage)},
	{"Checkbox", "Checkbox", "kit.Checkbox", "A box that is checked or not, on its own or in a Field with its label.", tela.C(checkboxPage)},
	{"ColorInput", "Color Input", "kit.ColorInput", "A button that shows a color and opens a color picker in a popover.", tela.C(colorInputPage)},
	{"ColorPicker", "Color Picker", "kit.ColorPicker", "An area for saturation and brightness, a hue slider, an optional alpha slider, a hex field and swatches.", tela.C(colorPickerPage)},
	{"ContextMenu", "Context Menu", "kit.ContextMenu[K]", "A menu that opens where the user right-clicks inside an area.", tela.C(contextMenuPage)},
	{"DatePicker", "Date Picker", "kit.DatePicker", "A button that shows a date and opens a calendar to choose it.", tela.C(datePickerPage)},
	{"DateRangePicker", "Date Range Picker", "kit.DateRangePicker", "A date picker for a span of dates, with two months in its popover.", tela.C(dateRangePickerPage)},
	{"Dialog", "Dialog", "kit.Dialog", "A modal window over the page: a trigger, a title, a description, content and a footer.", tela.C(dialogPage)},
	{"DropdownMenu", "Dropdown Menu", "kit.DropdownMenu[K]", "A menu of actions that opens from a button.", tela.C(dropdownMenuPage)},
	{"Field", "Field", "kit.Field", "A control with its label, help text and error, tied together so the label names the control.", tela.C(fieldPage)},
	{"FieldSet", "Field Set", "kit.FieldSet", "A group of fields under a legend, which names the group and a radio or toggle group inside it.", tela.C(fieldSetPage)},
	{"Input", "Input", "kit.Input", "A single-line text field, controlled or not.", tela.C(inputPage)},
	{"Label", "Label", "kit.Label", "The caption of a form control. Clicking it focuses the control it names.", tela.C(labelPage)},
	{"Menubar", "Menubar", "kit.Menubar[K]", "A row of menus, such as the File, Edit and View of an application.", tela.C(menubarPage)},
	{"Popover", "Popover", "kit.Popover", "A panel anchored to its trigger. It does not block the page.", tela.C(popoverPage)},
	{"Progress", "Progress", "kit.Progress", "A bar that shows how far a task is.", tela.C(progressPage)},
	{"RadioGroup", "Radio Group", "kit.RadioGroup[K]", "A set of options of which one is chosen, with the arrow keys, or drawn as you like with Render.", tela.C(radioGroupPage)},
	{"RangeCalendar", "Range Calendar", "kit.RangeCalendar", "A calendar that selects a span of dates in two presses.", tela.C(rangeCalendarPage)},
	{"Select", "Select", "kit.Select[K]", "A button that shows the chosen option and opens a list to choose another.", tela.C(selectPage)},
	{"Separator", "Separator", "kit.Separator", "A thin line that divides content, horizontal or vertical.", tela.C(separatorPage)},
	{"Skeleton", "Skeleton", "kit.Skeleton", "A placeholder that pulses while content loads.", tela.C(skeletonPage)},
	{"Slider", "Slider", "kit.Slider", "A number, or a range, picked by dragging a knob along a track.", tela.C(sliderPage)},
	{"Switch", "Switch", "kit.Switch", "An on/off control that slides.", tela.C(switchPage)},
	{"Table", "Table", "kit.Table[T]", "Rows of any type through columns, with sorting by a header and a sticky header and columns.", tela.C(tablePage)},
	{"Tabs", "Tabs", "kit.Tabs[K]", "A tab list with its panels, in the default pill or underlined, horizontal or vertical.", tela.C(tabsPage)},
	{"Textarea", "Textarea", "kit.Textarea", "A multi-line text field that grows with its text.", tela.C(textareaPage)},
	{"TitleBar", "Title Bar", "kit.TitleBar, kit.WindowControls", "The title bar of a window that draws its own: a drag area with a title, slots and the minimize, maximize and close buttons, in the look of Windows, GNOME or macOS.", tela.C(titleBarPage)},
	{"Toast", "Toast", "kit.Toaster, kit.UseToast", "Toasts that stack at a corner of the window, shown from any component or goroutine.", tela.C(toastPage)},
	{"Toggle", "Toggle", "kit.Toggle", "A button that stays pressed.", tela.C(togglePage)},
	{"ToggleGroup", "Toggle Group", "kit.ToggleGroup[K]", "A row of toggle buttons that share a choice: the segmented control and the formatting toolbar.", tela.C(toggleGroupPage)},
	{"Tooltip", "Tooltip", "kit.Tooltip", "A small label that explains its trigger, on hover or keyboard focus.", tela.C(tooltipPage)},
}
