package main

// Prefs are the settings the form edits. It is a plain comparable struct,
// so `prefs != saved` tells whether there is anything to save, and it can
// travel through a Setter that components compare in their props.
type Prefs struct {
	Email, Push, Digest, Marketing bool
	Theme, Density                 string
}

// Field names one editable field of Prefs.
type Field int

// The fields.
const (
	FieldEmail Field = iota
	FieldPush
	FieldDigest
	FieldMarketing
	FieldTheme
	FieldDensity
	// FieldSecurity is the always-on, locked switch of the notifications
	// card; it is not stored.
	FieldSecurity
)

// Default is the form's initial and saved state.
var Default = Prefs{Email: true, Push: true, Theme: "system", Density: "comfortable"}

// Bool returns a boolean field.
func (p Prefs) Bool(f Field) bool {
	switch f {
	case FieldEmail:
		return p.Email
	case FieldPush:
		return p.Push
	case FieldDigest:
		return p.Digest
	case FieldMarketing:
		return p.Marketing
	case FieldSecurity:
		return true
	}
	panic("settings: not a boolean field")
}

// Toggle returns p with the boolean field f flipped.
func (p Prefs) Toggle(f Field) Prefs {
	switch f {
	case FieldEmail:
		p.Email = !p.Email
	case FieldPush:
		p.Push = !p.Push
	case FieldDigest:
		p.Digest = !p.Digest
	case FieldMarketing:
		p.Marketing = !p.Marketing
	default:
		panic("settings: not a boolean field")
	}
	return p
}

// Choice returns the selected value of a radio field.
func (p Prefs) Choice(f Field) string {
	if f == FieldTheme {
		return p.Theme
	}
	return p.Density
}

// Choose returns p with the radio field f set to v.
func (p Prefs) Choose(f Field, v string) Prefs {
	if f == FieldTheme {
		p.Theme = v
	} else {
		p.Density = v
	}
	return p
}

// Notification is a row of the notifications card.
type Notification struct {
	Field       Field
	Title, Hint string
}

var notifications = []Notification{
	{FieldEmail, "Email notifications", "Receive a summary of activity in your workspace."},
	{FieldPush, "Push notifications", "Get notified about mentions and replies on your devices."},
	{FieldDigest, "Weekly digest", "A Monday morning recap of what your team shipped."},
	{FieldMarketing, "Product updates", "News about new features, tips and special offers."},
}

// Option is a choice of a radio group.
type Option struct {
	Value, Title, Hint string
}

var themes = []Option{
	{"light", "Light", ""},
	{"dark", "Dark", ""},
	{"system", "System", ""},
}

var densities = []Option{
	{"compact", "Compact", "Fit more on screen."},
	{"comfortable", "Comfortable", "Balanced spacing. The default."},
	{"spacious", "Spacious", "Room to breathe around everything."},
}
