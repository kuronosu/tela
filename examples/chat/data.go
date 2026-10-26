package main

import "time"

// Seed returns the conversations the app starts with, written relative to
// now so that "Today" and "Yesterday" are always right. The people are
// made up.
func Seed(now time.Time) Store {
	// A line of a thread: when (how long ago), who and what.
	type line struct {
		ago  time.Duration
		mine bool
		text string
	}
	thread := func(lines ...line) *Thread {
		t := &Thread{}
		for _, l := range lines {
			t.Messages = append(t.Messages, Message{Text: l.text, At: now.Add(-l.ago), Mine: l.mine, Read: l.mine})
		}
		return t
	}
	minute, hour := time.Minute, time.Hour
	them := func(ago time.Duration, text string) line { return line{ago, false, text} }
	me := func(ago time.Duration, text string) line { return line{ago, true, text} }

	person := func(id, name, initials, tone string, status Status) Person {
		return Person{ID: id, Name: name, Initials: initials, Tone: tone, Status: status}
	}

	return Store{
		Active: "maya",
		Today:  now,
		Convs: []Conv{
			{
				Person: person("maya", "Maya Chen", "MC", "bg-rose-100 text-rose-700 dark:bg-rose-500/20 dark:text-rose-300", Online),
				Thread: thread(
					them(31*hour, "Hey! Did you get a chance to look at the onboarding flow?"),
					them(31*hour-time.Minute, "I moved the progress dots to the top, it feels less crowded."),
					me(30*hour, "Just did. The new layout is much cleaner 👏"),
					me(30*hour-2*minute, "One question: should the skip link stay visible on the last step?"),
					them(29*hour, "Good catch, I'll hide it there."),
					them(3*hour+20*minute, "Pushed the updated frames to Figma, take a look when you can."),
					me(3*hour, "Looks great. The empty state illustration is lovely."),
					them(2*hour+58*minute, "Thanks! Sofia drew it. She says hi 🙂"),
					them(35*minute, "Are we still on for the design review at 3?"),
					them(34*minute, "I can walk you through the changes if that helps."),
				),
			},
			{
				Person: person("dev", "Dev Patel", "DP", "bg-sky-100 text-sky-700 dark:bg-sky-500/20 dark:text-sky-300", Online),
				Thread: thread(
					me(5*hour, "Can you take a look at the flaky upload test?"),
					them(4*hour, "On it. I think it's a race in the retry logic."),
					them(12*minute, "Fixed, and staging is green again."),
					them(11*minute, "I'll ship it at 4 if nobody objects."),
				),
				Unread: 2,
			},
			{
				Person: person("sofia", "Sofia Rossi", "SR", "bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-300", Away),
				Thread: thread(
					me(26*hour, "Could you send the illustrations as SVG?"),
					them(2*hour, "Sent you the final exports 📦"),
				),
				Unread: 1,
			},
			{
				Person: person("jonas", "Jonas Weber", "JW", "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300", Offline),
				Thread: thread(
					them(27*hour, "A customer asks if exports can include comments."),
					me(26*hour+10*minute, "Let me check with the team and get back to you."),
				),
			},
			{
				Person: person("amara", "Amara Okafor", "AO", "bg-violet-100 text-violet-700 dark:bg-violet-500/20 dark:text-violet-300", Online),
				Thread: thread(
					them(50*hour, "The launch post is scheduled for Thursday 🚀"),
					me(49*hour, "Perfect, thank you!"),
				),
			},
			{
				Person: person("liam", "Liam O'Brien", "LO", "bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-300", Offline),
				Thread: thread(
					me(75*hour, "Did the invoice for October go out?"),
					them(74*hour, "Invoice approved, thanks for the reminder!"),
				),
			},
			{
				Person: person("yuki", "Yuki Tanaka", "YT", "bg-teal-100 text-teal-700 dark:bg-teal-500/20 dark:text-teal-300", Away),
				Thread: thread(
					them(8*24*hour, "Dashboard numbers look right now."),
				),
			},
		},
	}
}

// scripts are the lines each person answers with, in order.
var scripts = map[string][]string{
	"maya": {
		"Perfect, I'll book a room and send the invite.",
		"Also, I'm adding dark mode frames to the file today.",
		"Haha, you read my mind 😄",
	},
	"dev": {
		"Merged. I'll keep an eye on the logs after the deploy.",
		"No rush, I'll pick it up after lunch.",
	},
	"sofia": {
		"Of course! I'll send the SVGs in a minute.",
		"Let me know if you want another palette.",
	},
	"jonas": {
		"Thanks, that would help a lot.",
		"I'll tell the customer to expect it next week.",
	},
	"amara": {
		"Great. I'll share the draft in the channel.",
		"Thursday at nine, then!",
	},
	"liam": {
		"All good on my side.",
		"I'll send the receipts on Monday.",
	},
	"yuki": {
		"Glad it works. The weekly report is next.",
		"I'll refresh the data tonight.",
	},
}
