package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/pricing/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
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

// PlanCard is one pricing tier, a kit.Card. The highlighted plan is the
// inverted card (bg-primary) with a floating badge; the others are plain
// cards.
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

	class, muted := "relative w-[340px] gap-8", "text-muted-foreground"
	check := "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300"
	cta := kit.Outline
	if plan.Highlight {
		class += " border-primary bg-primary text-primary-foreground shadow-xl"
		muted = "text-primary-foreground/70"
		check = "bg-primary-foreground/15 text-primary-foreground"
		cta = kit.Secondary
	}

	return Fragment(kit.Card{
		Class:            class,
		TitleClass:       "text-lg",
		DescriptionClass: muted,
		TitleLevel:       3,
		Title:            Text(plan.Name),
		Description:      Text(plan.Description),
		ContentClass:     "flex flex-col gap-6",
		Children: Fragment(
			If(plan.Highlight, kit.Badge{
				Class:    "absolute -top-3 left-1/2 -translate-x-1/2 border-2 border-background px-3 py-1 font-semibold",
				Children: Text("Most popular"),
			}),
			Div(Class("flex flex-col gap-1"),
				Div(Class("flex items-baseline gap-1.5"),
					Span(Class("text-5xl font-bold tracking-tight tabular-nums"), Textf("$%d", price)),
					Span(Class("text-sm", muted), Text("/ month")),
				),
				P(Class("text-sm", muted), Text(billing)),
			),
			Div(Class("flex flex-col gap-3"),
				Map(plan.Features, func(f string) tela.Node {
					return Div(Key(f), Class("flex items-center gap-3 text-sm"),
						Span(Class("flex size-5 shrink-0 items-center justify-center rounded-full", check),
							Icon(icons.Check, Class("size-3 stroke-[3]"))),
						Text(f),
					)
				}),
			),
		),
		Footer: kit.Button{Variant: cta, Size: kit.Lg, Class: "w-full", Children: Text(plan.CTA)},
	})
}

// BillingToggle is the monthly/yearly choice: a kit.ToggleGroup, the
// segmented control, with a badge on the yearly option.
type BillingToggle struct {
	Yearly bool
	Set    tela.Setter[bool]
}

func (p BillingToggle) Build() tela.Node {
	period := "monthly"
	if p.Yearly {
		period = "yearly"
	}
	set := p.Set
	return Fragment(kit.ToggleGroup[string]{
		AriaLabel: "Billing period", Variant: kit.Outline, DisallowEmptySelection: true,
		Value: &period, OnValueChange: func(v string) { set.Set(v == "yearly") },
		Items: []kit.ToggleGroupItem[string]{
			{Value: "monthly", Label: "Monthly"},
			{Value: "yearly", Children: Fragment(Text("Yearly"),
				kit.Badge{Variant: kit.Secondary, Class: "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300", Children: Text("Save 20%")})},
		},
	})
}
