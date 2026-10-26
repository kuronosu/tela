package main

import (
	"slices"
	"strconv"
	"time"
)

// The model of the app: plain data and one pure reducer, no UI. The whole
// app state is a Store in tela.UseReducer, so a draft, a selection and a
// reply that arrives are all actions on it, and the tests can drive the
// model without a window.
//
// Components get small comparable pieces of it (a Conv, a Person, a *Thread)
// as props, so one skips its build while its piece is the same. A Thread
// holds a slice, which is not comparable, so it is immutable and handed
// around as a pointer: sending a message makes a new Thread, and a pointer
// compares by identity. That is what lets typing in the composer, which
// changes the Store on every key, leave the message list alone.

// Status is whether a person is around.
type Status string

// The statuses.
const (
	Online  Status = "online"
	Away    Status = "away"
	Offline Status = "offline"
)

// Person is who a conversation is with.
type Person struct {
	ID, Name, Initials string
	// Tone is the classes of the avatar's initials: a tinted disc of
	// their own, so people are told apart at a glance.
	Tone   string
	Status Status
}

// Message is one message of a thread.
type Message struct {
	Text string
	At   time.Time
	// Mine is a message you sent; Read says the other side saw it.
	Mine, Read bool
}

// Thread is the messages of a conversation, oldest first. It is immutable:
// Append and ReadAll return a new Thread.
type Thread struct{ Messages []Message }

// Append returns the thread with m after its messages.
func (t *Thread) Append(m Message) *Thread {
	return &Thread{Messages: append(slices.Clone(t.Messages), m)}
}

// ReadAll returns the thread with every message of yours marked as read.
func (t *Thread) ReadAll() *Thread {
	msgs := slices.Clone(t.Messages)
	for i := range msgs {
		msgs[i].Read = true
	}
	return &Thread{Messages: msgs}
}

// Last returns the newest message, if there is one.
func (t *Thread) Last() (Message, bool) {
	if len(t.Messages) == 0 {
		return Message{}, false
	}
	return t.Messages[len(t.Messages)-1], true
}

// Conv is a conversation: a person, their thread and what you are doing
// with it. It is comparable, so a row of the sidebar skips its build until
// its own conversation changes.
type Conv struct {
	Person
	Thread *Thread
	// Draft is what you typed and did not send; it survives switching to
	// another conversation.
	Draft  string
	Unread int
	Muted  bool
	// Typing says the other side is writing a reply; Replies counts the
	// replies so far, to pick the next line of the script.
	Typing  bool
	Replies int
}

// Store is the whole state of the app.
type Store struct {
	Convs []Conv
	// Active is the ID of the open conversation.
	Active string
	// Today is the day the app started, for "Today" and "Yesterday".
	Today time.Time
}

// Find returns the conversation with the given ID.
func (s Store) Find(id string) Conv {
	i := slices.IndexFunc(s.Convs, func(c Conv) bool { return c.ID == id })
	if i < 0 {
		return Conv{Thread: &Thread{}}
	}
	return s.Convs[i]
}

// Unread is the number of unread messages in conversations that are not
// muted.
func (s Store) Unread() (n int) {
	for _, c := range s.Convs {
		if !c.Muted {
			n += c.Unread
		}
	}
	return n
}

// Pending names the conversation whose reply is due: the first one the
// other side is typing in. N is the length of its thread, so sending
// another message while they type changes the value and restarts the wait.
type Pending struct {
	ID string
	N  int
}

// Pending returns the reply that is due next, or the zero Pending.
func (s Store) Pending() Pending {
	for _, c := range s.Convs {
		if c.Typing {
			return Pending{c.ID, len(c.Thread.Messages)}
		}
	}
	return Pending{}
}

// Kind is what an Action does.
type Kind int

// The kinds of action.
const (
	Select Kind = iota
	SetDraft
	Send
	Reply
	ToggleMute
	Clear
)

// Action is one change of the Store. At is when a message was sent or
// answered: the root stamps it, so the reducer stays pure and the tests
// choose the clock.
type Action struct {
	Kind Kind
	ID   string // the conversation
	Text string // the draft or the message
	At   time.Time
}

// reduce applies an action to a copy of the store.
func reduce(s Store, a Action) Store {
	s.Convs = slices.Clone(s.Convs)
	i := slices.IndexFunc(s.Convs, func(c Conv) bool { return c.ID == a.ID })
	if i < 0 {
		return s
	}
	c := &s.Convs[i]
	switch a.Kind {
	case Select:
		s.Active, c.Unread = c.ID, 0
	case SetDraft:
		c.Draft = a.Text
	case Send:
		c.Thread = c.Thread.Append(Message{Text: a.Text, At: a.At, Mine: true})
		c.Draft, c.Typing = "", true
	case Reply:
		if !c.Typing {
			return s
		}
		lines := scripts[c.ID]
		c.Thread = c.Thread.ReadAll().Append(Message{Text: lines[c.Replies%len(lines)], At: a.At})
		c.Typing, c.Replies = false, c.Replies+1
		if c.ID != s.Active {
			c.Unread++
		}
	case ToggleMute:
		c.Muted = !c.Muted
	case Clear:
		c.Thread, c.Typing = &Thread{}, false
	}
	return s
}

// Item is one row of a message list: a day separator (Day is set) or a
// message with its place in a group of consecutive messages from the same
// side. It is a component, comparable like everything else here, so
// appending a message builds only the new row and the one before it.
type Item struct {
	Key string
	Day string
	// From names the sender of a message: "You" or the person.
	From        string
	Msg         Message
	First, Last bool
}

// Items lays a thread out: a separator before the first message of every
// day, and the messages of one side within five minutes of each other
// grouped, so only the last of a group shows its time.
func (t *Thread) Items(person string, today time.Time) []Item {
	msgs := t.Messages
	joined := func(i int) bool { // does message i continue the group of i-1?
		if i <= 0 || i >= len(msgs) {
			return false
		}
		a, b := msgs[i-1], msgs[i]
		return a.Mine == b.Mine && daysBetween(a.At, b.At) == 0 && b.At.Sub(a.At) < 5*time.Minute
	}
	var items []Item
	for i, m := range msgs {
		if i == 0 || daysBetween(msgs[i-1].At, m.At) != 0 {
			items = append(items, Item{Key: "day-" + m.At.Format("20060102"), Day: dayLabel(m.At, today)})
		}
		from := person
		if m.Mine {
			from = "You"
		}
		items = append(items, Item{Key: "msg-" + strconv.Itoa(i), From: from, Msg: m, First: !joined(i), Last: !joined(i + 1)})
	}
	return items
}

// daysBetween counts calendar days from a to b, in their own zone.
func daysBetween(a, b time.Time) int {
	noon := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	}
	return int(noon(b).Sub(noon(a)).Hours() / 24)
}

// dayLabel names a day for a separator: Today, Yesterday, or the date.
func dayLabel(t, today time.Time) string {
	switch daysBetween(t, today) {
	case 0:
		return "Today"
	case 1:
		return "Yesterday"
	}
	return t.Format("Monday, January 2")
}

// shortTime is the time of a conversation's last message in the sidebar:
// the hour today, then Yesterday, a weekday within the week, a date.
func shortTime(t, today time.Time) string {
	switch d := daysBetween(t, today); {
	case d == 0:
		return t.Format("3:04 PM")
	case d == 1:
		return "Yesterday"
	case d < 7:
		return t.Format("Mon")
	}
	return t.Format("Jan 2")
}
