package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Plan is the data of one tier. Plans hold a slice, so they are not
// comparable: the cards receive the plan's ID in their props and look the
// plan up.
type Plan struct {
	ID, Name, Description string
	Monthly, Yearly       int // price per month, billed monthly or yearly
	Highlight             bool
	CTA                   string
	Features              []string
}

var plans = []Plan{
	{
		ID: "starter", Name: "Starter", Description: "For individuals and side projects.",
		Monthly: 0, Yearly: 0, CTA: "Get started",
		Features: []string{"1 project", "2 GB of storage", "Community support", "Basic analytics", "Public API access"},
	},
	{
		ID: "pro", Name: "Pro", Description: "For growing teams that ship fast.",
		Monthly: 29, Yearly: 24, Highlight: true, CTA: "Start free trial",
		Features: []string{"Unlimited projects", "100 GB of storage", "Priority email support", "Advanced analytics", "Custom domains"},
	},
	{
		ID: "enterprise", Name: "Enterprise", Description: "For organizations with advanced needs.",
		Monthly: 99, Yearly: 79, CTA: "Contact sales",
		Features: []string{"Everything in Pro", "SSO and audit logs", "99.99% uptime SLA", "Dedicated success manager", "On-premise deployment"},
	},
}

func planByID(id string) Plan {
	for _, p := range plans {
		if p.ID == id {
			return p
		}
	}
	panic("pricing: unknown plan " + id)
}

// PlanCard is one pricing tier. The highlighted plan is a dark card with a
// badge; the others are white.
type PlanCard struct {
	ID     string
	Yearly bool
}

func (p PlanCard) Build() tela.Node {
	plan := planByID(p.ID)
	price := plan.Monthly
	if p.Yearly {
		price = plan.Yearly
	}
	billing := "Billed monthly"
	switch {
	case price == 0:
		billing = "Free forever"
	case p.Yearly:
		billing = fmt.Sprintf("Billed $%d per year", price*12)
	}

	surface := "border border-zinc-200 bg-white text-zinc-950 shadow-sm dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-50"
	muted := "text-zinc-500 dark:text-zinc-400"
	cta := Class("border border-zinc-200 bg-white text-zinc-900 shadow-xs hover:bg-zinc-100 active:bg-zinc-200",
		"dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-50 dark:hover:bg-zinc-700")
	check := "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300"
	divider := "bg-zinc-100 dark:bg-zinc-800"
	if plan.Highlight {
		surface = "border border-transparent bg-linear-to-b from-zinc-800 to-zinc-950 text-white shadow-2xl dark:border-white/15"
		muted = "text-zinc-400"
		cta = Class("bg-white text-zinc-900 shadow-sm hover:bg-zinc-200 active:bg-zinc-300")
		check = "bg-white/15 text-white"
		divider = "bg-white/10"
	}

	return Div(Class("relative flex w-[340px] flex-col gap-6 rounded-2xl p-8", surface),
		If(plan.Highlight, Div(
			Class("absolute -top-3 left-1/2 -translate-x-1/2 rounded-full bg-linear-to-r from-violet-500 to-indigo-500 px-3 py-1 text-xs font-semibold whitespace-nowrap text-white shadow-md"),
			Text("Most popular"))),
		Div(Class("flex flex-col gap-1.5"),
			H3(Class("text-lg font-semibold"), Text(plan.Name)),
			P(Class("text-sm", muted), Text(plan.Description)),
		),
		Div(Class("flex flex-col gap-1"),
			Div(Class("flex items-baseline gap-1.5"),
				Span(Class("text-5xl font-bold tracking-tight tabular-nums"), Textf("$%d", price)),
				Span(Class("text-sm", muted), Text("/ month")),
			),
			P(Class("text-sm", muted), Text(billing)),
		),
		Button(Class("flex h-10 items-center justify-center rounded-lg text-sm font-medium transition-colors"), cta,
			Text(plan.CTA)),
		Div(Class("h-px", divider)),
		Div(Class("flex flex-col gap-3"),
			Map(plan.Features, func(f string) tela.Node {
				return Div(Key(f), Class("flex items-center gap-3 text-sm"),
					Check{Class: check},
					Text(f),
				)
			}),
		),
	)
}

// Check is a check mark in a circle, drawn with the right and bottom
// borders of a rotated box. Class colors the circle and, through
// border-current, the mark.
type Check struct {
	Class string
}

func (c Check) Build() tela.Node {
	return Div(Class("flex size-5 shrink-0 items-center justify-center rounded-full", c.Class),
		Div(Class("-mt-0.5 h-2.5 w-1.5 rotate-45 border-r-2 border-b-2 border-current")),
	)
}

// BillingToggle is "Monthly [switch] Yearly": the labels and the switch
// are all buttons that set the period.
type BillingToggle struct {
	Yearly bool
	Set    tela.Setter[bool]
}

func (p BillingToggle) Build() tela.Node {
	on, off := "text-zinc-950 dark:text-zinc-50", "text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200"
	monthly, yearly := on, off
	track, knob := "justify-start bg-zinc-300 hover:bg-zinc-400 dark:bg-zinc-700 dark:hover:bg-zinc-600", "bg-white dark:bg-zinc-300"
	if p.Yearly {
		monthly, yearly = off, on
		track, knob = "justify-end bg-zinc-900 hover:bg-zinc-700 dark:bg-indigo-500 dark:hover:bg-indigo-400", "bg-white"
	}
	return Div(Class("flex items-center gap-3 text-sm font-medium"),
		Button(Class("transition-colors", monthly), OnClick(func() { p.Set.Set(false) }), Text("Monthly")),
		Button(Class("flex h-6 w-11 items-center rounded-full p-0.5 transition-colors", track),
			OnClick(tela.Toggle(p.Set)),
			Div(Class("size-5 rounded-full shadow-sm", knob)),
		),
		Button(Class("flex items-center gap-2 transition-colors", yearly), OnClick(func() { p.Set.Set(true) }),
			Text("Yearly"),
			Span(Class("rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300"),
				Text("Save 20%")),
		),
	)
}
