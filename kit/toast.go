// Portions of this file follow shadcn/ui, sonner.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// Portions also follow Sonner's default styles and stacking rule
// (https://github.com/emilkowalski/sonner, 2.0.7), Copyright (c) 2023 Emil Kowalski,
// licensed under the MIT License.
// Icons embedded in this file are copied unaltered from Lucide
// (https://lucide.dev, lucide-static 1.61.0), Copyright (c) 2026 Lucide Icons and
// Contributors, licensed under the ISC License (the icons derived from Feather are
// MIT, Copyright (c) 2013-present Cole Bemis).
// See THIRD_PARTY.md.

package kit

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// The icons Toaster draws, from Lucide (ISC, see THIRD_PARTY.md), as
// shadcn/ui's Sonner wrapper chooses them; each is replaceable with
// Toaster.Icons and CloseIcon.
var (
	toastCircleCheck  = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></svg>`}
	toastInfo         = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>`}
	toastTriangleWarn = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>`}
	toastOctagonX     = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15 9-6 6"/><path d="M2.586 16.726A2 2 0 0 1 2 15.312V8.688a2 2 0 0 1 .586-1.414l4.688-4.688A2 2 0 0 1 8.688 2h6.624a2 2 0 0 1 1.414.586l4.688 4.688A2 2 0 0 1 22 8.688v6.624a2 2 0 0 1-.586 1.414l-4.688 4.688a2 2 0 0 1-1.414.586H8.688a2 2 0 0 1-1.414-.586z"/><path d="m9 9 6 6"/></svg>`}
	toastLoader       = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>`}
	toastX            = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`}
)

// ToastPosition is the corner or edge of the window a [Toaster] puts its
// toasts at. The zero value is ToastBottomRight. Left and right are the
// window's, whatever the direction, as sonner's positions are (the toast's
// own layout, its icon, action and close button, follows the direction).
type ToastPosition string

const (
	ToastTopLeft      ToastPosition = "top-left"
	ToastTopCenter    ToastPosition = "top-center"
	ToastTopRight     ToastPosition = "top-right"
	ToastBottomLeft   ToastPosition = "bottom-left"
	ToastBottomCenter ToastPosition = "bottom-center"
	ToastBottomRight  ToastPosition = "bottom-right"
)

// ToastForever as a duration keeps a toast until it is dismissed.
const ToastForever time.Duration = -1

// ToastStyle is the recipe of [Toaster] (shadcn/ui's Sonner wrapper over
// sonner's default styles, sonner 2.x). The root element of a Toaster is
// its viewport: the box at the edge of the window the toasts stack in.
type ToastStyle struct {
	// Root is the viewport, Fixed or Inline is added to it (the viewport is
	// fixed to the window, or absolute inside the nearest positioned
	// ancestor for an Inline Toaster) and Positions say where.
	Root, Fixed, Inline string
	Positions           map[ToastPosition]string
	// Item is one toast: all of them are absolute at the viewport's edge, and
	// the Toaster moves them with a translation and a scale. Sides add the
	// edge ("top" or "bottom") and x the side they are laid against.
	Item  string
	Sides map[string]string
	// Enter and Exit are the classes an entering toast starts from and an
	// exiting one goes to, per edge.
	Enter, Exit map[string]string
	// Hidden is added to the toasts beyond VisibleToasts.
	Hidden string
	// Body is the row inside a toast that holds the icon, the text and the
	// action; Collapsed is added to the body of a toast behind the first
	// while the stack is collapsed (its content is invisible, only its card
	// peeks out).
	Body, Collapsed string
	// Bridge is the invisible strip above an expanded toast that keeps the
	// pointer over the viewport while it crosses the gap between two toasts
	// (sonner's ::after).
	Bridge string
	// Icon is the box of a toast's icon, IconSvg the drawing, and Spinner is
	// added to the loading icon.
	Icon, IconSvg, Spinner string
	Content                string
	Title                  string
	Description            string
	// Action is the toast's button, Close the round button of a Toaster
	// with a CloseButton and CloseIcon its icon.
	Action, Close, CloseIcon string
}

func init() {
	defaultStyle.Toast = ToastStyle{
		Root:   "z-[999999999] w-[356px] outline-none",
		Fixed:  "fixed",
		Inline: "absolute",
		Positions: map[ToastPosition]string{
			ToastTopLeft:      "top-6 left-6",
			ToastTopCenter:    "top-6 left-1/2 -translate-x-1/2",
			ToastTopRight:     "top-6 right-6",
			ToastBottomLeft:   "bottom-6 left-6",
			ToastBottomCenter: "bottom-6 left-1/2 -translate-x-1/2",
			ToastBottomRight:  "bottom-6 right-6",
		},
		Item: "absolute start-0 box-border flex w-full items-center gap-1.5 rounded-lg border border-border bg-popover p-4 text-[13px] text-popover-foreground " +
			"shadow-[0_4px_12px_rgba(0,0,0,0.1)] transition-[transform,opacity,height,box-shadow] duration-[400ms] outline-none focus-visible:ring-2 focus-visible:ring-black/20",
		Sides: map[string]string{
			"top":    "top-0",
			"bottom": "bottom-0",
		},
		Enter: map[string]string{
			"top":    "starting:-translate-y-full starting:opacity-0",
			"bottom": "starting:translate-y-full starting:opacity-0",
		},
		Exit: map[string]string{
			"top":    "-translate-y-full opacity-0 duration-200",
			"bottom": "translate-y-full opacity-0 duration-200",
		},
		Hidden:      "pointer-events-none opacity-0",
		Body:        "flex w-full items-center gap-1.5 transition-opacity duration-[400ms]",
		Collapsed:   "opacity-0",
		Bridge:      "absolute bottom-full start-0 h-[15px] w-full",
		Icon:        "relative -ms-[3px] me-1 flex size-4 shrink-0 items-center justify-start",
		IconSvg:     "-ms-px size-4 shrink-0",
		Spinner:     "animate-spin",
		Content:     "flex flex-col gap-0.5",
		Title:       "leading-normal font-medium",
		Description: "leading-[1.4] font-normal text-[#3f3f3f] dark:text-[#e8e8e8]",
		Action: "ms-auto flex h-6 shrink-0 cursor-pointer items-center rounded bg-popover-foreground px-2 text-xs font-medium text-popover outline-none " +
			"focus-visible:ring-2 focus-visible:ring-black/40",
		Close: "absolute top-0 start-0 z-10 flex size-5 -translate-x-[35%] -translate-y-[35%] rtl:translate-x-[35%] cursor-pointer items-center justify-center rounded-full border border-[#ededed] " +
			"bg-popover p-0 text-[#171717] outline-none transition-colors hover:border-[#e8e8e8] hover:bg-[#f8f8f8] focus-visible:ring-2 focus-visible:ring-black/20 " +
			"dark:border-border dark:text-popover-foreground dark:hover:border-[#404040] dark:hover:bg-[#1f1f1f]",
		CloseIcon: "size-3 stroke-[1.5]",
	}
}

// ToastID names one toast of a [Toaster]: [ToastHandle].Show returns it, and
// Dismiss and [ToastReplace] take it.
type ToastID uint64

type toastKind int

const (
	toastNormal toastKind = iota
	toastSuccess
	toastError
	toastInfo2
	toastWarning
	toastLoading
)

// toastData is a toast as the store holds it.
type toastData struct {
	id          ToastID
	kind        toastKind
	title       string
	description string
	action      string
	onAction    func()
	duration    time.Duration // 0: the Toaster's
	onDismiss   func()
	onAutoClose func()
	// gen counts the updates of the toast: a new one restarts its timer.
	gen int
}

// ToastOption customizes a toast shown with [ToastHandle].Show and its
// variants.
type ToastOption func(*toastData)

// ToastDescription adds a second, lighter line under the title.
func ToastDescription(text string) ToastOption {
	return func(t *toastData) { t.description = text }
}

// ToastAction adds a button to the toast. Clicking it calls onClick and
// dismisses the toast.
func ToastAction(label string, onClick func()) ToastOption {
	return func(t *toastData) { t.action, t.onAction = label, onClick }
}

// ToastDuration sets how long the toast stays: after it the toast is
// dismissed. [ToastForever] keeps it until it is dismissed by hand. The
// default is the Toaster's Duration (4 seconds), and a loading toast never
// closes by itself. The time is the window's frame clock, so it stops while
// the pointer rests over the toasts.
func ToastDuration(d time.Duration) ToastOption {
	return func(t *toastData) { t.duration = d }
}

// ToastReplace makes the toast replace the one with this id, in the same
// place and with its timer restarted, as sonner's id option does: a
// loading toast becomes a success or an error when the work is over. When
// no toast has the id, a new one is shown.
func ToastReplace(id ToastID) ToastOption {
	return func(t *toastData) { t.id = id }
}

// ToastOnDismiss calls f, on the UI thread, when the toast is dismissed by
// the user or by [ToastHandle].Dismiss.
func ToastOnDismiss(f func()) ToastOption {
	return func(t *toastData) { t.onDismiss = f }
}

// ToastOnAutoClose calls f, on the UI thread, when the toast closes
// because its time ran out.
func ToastOnAutoClose(f func()) ToastOption {
	return func(t *toastData) { t.onAutoClose = f }
}

// toastConfig is what a Toaster tells its store on every build.
type toastConfig struct {
	duration time.Duration
}

// toastStore is the state a Toaster shares with the handles of UseToast. It
// is safe from any goroutine: a mutex guards the toasts, and every change
// asks the Toaster to rebuild through a state setter, which routes through
// the tree like tela.Post.
type toastStore struct {
	bump tela.Setter[int]

	mu      sync.Mutex
	cfg     toastConfig
	next    ToastID
	toasts  []toastData // newest first
	heights map[ToastID]float32
}

// changed asks the Toaster to build again.
func (s *toastStore) changed() { s.bump.Update(func(n int) int { return n + 1 }) }

func (s *toastStore) setConfig(c toastConfig) {
	s.mu.Lock()
	s.cfg = c
	s.mu.Unlock()
}

func (s *toastStore) config() toastConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// snapshot returns the toasts (newest first) and their measured heights.
func (s *toastStore) snapshot() (toasts []toastData, heights map[ToastID]float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	toasts = append(toasts, s.toasts...)
	heights = make(map[ToastID]float32, len(s.heights))
	for k, v := range s.heights {
		heights[k] = v
	}
	return toasts, heights
}

func (s *toastStore) show(kind toastKind, title string, opts []ToastOption) ToastID {
	t := toastData{kind: kind, title: title}
	for _, o := range opts {
		if o != nil {
			o(&t)
		}
	}
	s.mu.Lock()
	replaced := false
	if t.id != 0 {
		for i := range s.toasts {
			if s.toasts[i].id == t.id {
				t.gen = s.toasts[i].gen + 1
				s.toasts[i] = t
				replaced = true
				break
			}
		}
	}
	if !replaced {
		if t.id == 0 {
			s.next++
			t.id = s.next
		} else if t.id > s.next {
			// ToastReplace of an id no toast has: reserve it, so that the
			// automatic ids never reach it.
			s.next = t.id
		}
		s.toasts = append([]toastData{t}, s.toasts...)
	}
	id := t.id
	s.mu.Unlock()
	s.changed()
	return id
}

// remove takes the toast out and returns it.
func (s *toastStore) remove(id ToastID) (toastData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.toasts {
		if t.id == id {
			s.toasts = append(s.toasts[:i:i], s.toasts[i+1:]...)
			delete(s.heights, id)
			return t, true
		}
	}
	return toastData{}, false
}

func (s *toastStore) dismiss(id ToastID, auto bool) {
	t, ok := s.remove(id)
	if !ok {
		return
	}
	s.changed()
	cb := t.onDismiss
	if auto {
		cb = t.onAutoClose
	}
	if cb != nil {
		tela.Post(cb)
	}
}

func (s *toastStore) dismissAll() {
	s.mu.Lock()
	gone := s.toasts
	s.toasts, s.heights = nil, nil
	s.mu.Unlock()
	if len(gone) == 0 {
		return
	}
	s.changed()
	for _, t := range gone {
		if t.onDismiss != nil {
			tela.Post(t.onDismiss)
		}
	}
}

// setHeight records the measured height of a toast, in logical px.
func (s *toastStore) setHeight(id ToastID, px float32) {
	s.mu.Lock()
	exists := false
	for _, t := range s.toasts {
		if t.id == id {
			exists = true
		}
	}
	if !exists || math.Abs(float64(s.heights[id]-px)) < 0.01 {
		s.mu.Unlock()
		return
	}
	if s.heights == nil {
		s.heights = map[ToastID]float32{}
	}
	s.heights[id] = px
	s.mu.Unlock()
	s.changed()
}

// toastCtx carries the store of the Toaster above to UseToast.
var toastCtx = tela.CreateContext[*toastStore](nil)

// ToastHandle shows and dismisses the toasts of the [Toaster] above the
// component that called [UseToast]. It is a small comparable value, so it
// can sit in a struct component's fields without making it rebuild, and
// every method is safe from any goroutine: an HTTP callback or a worker can
// call Success when it is done.
type ToastHandle struct{ s *toastStore }

func (t ToastHandle) store() *toastStore {
	if t.s == nil {
		panic("kit: this ToastHandle is the zero value; get one with kit.UseToast() in a component below a kit.Toaster")
	}
	return t.s
}

// UseToast returns the handle of the nearest [Toaster] above the component.
// It is a hook. It panics when there is none: put kit.Toaster{Children: …}
// around the app, once.
//
//	func Save() tela.Node {
//	    toast := kit.UseToast()
//	    return kit.Button{OnClick: func() {
//	        toast.Success("Saved", kit.ToastDescription("Your changes are live."))
//	    }, Children: h.Text("Save")}
//	}
func UseToast() ToastHandle {
	s := tela.UseContext(toastCtx)
	if s == nil {
		panic("kit.UseToast: there is no kit.Toaster above this component; wrap the app in kit.Toaster{Children: app} (once, near the root)")
	}
	return ToastHandle{s}
}

// Show shows a toast with the title and returns its id. It is safe from
// any goroutine.
//
//	id := toast.Show("Event created", kit.ToastDescription("Sunday at 9:00 AM"),
//	    kit.ToastAction("Undo", undo))
func (t ToastHandle) Show(title string, opts ...ToastOption) ToastID {
	return t.store().show(toastNormal, title, opts)
}

// Success shows a toast with a check icon.
func (t ToastHandle) Success(title string, opts ...ToastOption) ToastID {
	return t.store().show(toastSuccess, title, opts)
}

// Error shows a toast with an error icon. It is announced as an alert.
func (t ToastHandle) Error(title string, opts ...ToastOption) ToastID {
	return t.store().show(toastError, title, opts)
}

// Info shows a toast with an info icon.
func (t ToastHandle) Info(title string, opts ...ToastOption) ToastID {
	return t.store().show(toastInfo2, title, opts)
}

// Warning shows a toast with a warning icon.
func (t ToastHandle) Warning(title string, opts ...ToastOption) ToastID {
	return t.store().show(toastWarning, title, opts)
}

// Loading shows a toast with a spinner that stays until it is dismissed or
// replaced (see [ToastReplace]).
//
//	id := toast.Loading("Uploading…")
//	// … later, from any goroutine:
//	toast.Success("Uploaded", kit.ToastReplace(id))
func (t ToastHandle) Loading(title string, opts ...ToastOption) ToastID {
	return t.store().show(toastLoading, title, opts)
}

// Dismiss closes the toast with the id; an id that is gone is ignored.
func (t ToastHandle) Dismiss(id ToastID) { t.store().dismiss(id, false) }

// DismissAll closes every toast.
func (t ToastHandle) DismissAll() { t.store().dismissAll() }

// ToastIcons are the icons of the toast kinds; a zero Icon keeps shadcn's.
type ToastIcons struct {
	Success, Info, Warning, Error, Loading tela.Icon
}

// Toaster shows the toasts of its app: shadcn/ui's Sonner. It goes once,
// around the app, and the components below it show toasts with a handle
// from [UseToast]:
//
//	tela.Main(tela.App{Title: "Notes", Root: kit.Toaster{Children: Notes{}}, Theme: &theme})
//
//	toast := kit.UseToast()
//	toast.Success("Saved")
//
// The toasts stack at a corner of the window (Position; bottom right by
// default). The newest is in front, with the ones before it peeking out
// behind it, scaled down; at most VisibleToasts (3) show. While the pointer
// is over the toasts they fan out to show each in full and their timers
// stop, and they go on from where they were when it leaves, as sonner
// does. A toast closes by itself after its Duration, 4 seconds by default;
// the clock is the window's frame clock, so telatest's Advance moves it
// and an app with only a waiting toast wakes up no more often than every
// 100 ms. Toasts enter and leave sliding from the edge of the window.
//
// Toasts are announced to assistive technologies: each is a "status" and an
// error an "alert".
//
// Compared with sonner: there is no swipe to dismiss, no promise helper
// (show a Loading toast and replace it), no rich colors, and the toasts do
// not change layout on narrow windows.
type Toaster struct {
	// Position is where the toasts stack: ToastBottomRight (the zero
	// value), ToastBottomLeft, ToastBottomCenter, ToastTopRight,
	// ToastTopLeft or ToastTopCenter.
	Position ToastPosition
	// Duration is how long a toast stays by default, 4 seconds when zero;
	// ToastForever keeps toasts until they are dismissed.
	Duration time.Duration
	// VisibleToasts is how many toasts show at once, 3 when zero.
	VisibleToasts int
	// Expand keeps the stack fanned out, as when the pointer is over it.
	Expand bool
	// CloseButton gives every toast a close button.
	CloseButton bool
	// Inline draws the toasts inside the nearest positioned ancestor of the
	// Toaster instead of in a layer fixed to the window: a toaster for a
	// panel or a preview. The position classes then place it in that box.
	Inline bool
	// Icons replace the icons of the kinds; CloseIcon the one of the close
	// button.
	Icons     ToastIcons
	CloseIcon tela.Icon
	// Children is the app: the components below that call UseToast.
	Children tela.Component

	// Class is added after the recipe's Root classes (the viewport's);
	// ToastClass is added to every toast.
	Class      string
	ToastClass string
	// Props are more props for the viewport element.
	Props tela.Child
}

const (
	toastGap      = 14 // px between toasts when expanded, and the lift of a stacked one
	toastTick     = 100 * time.Millisecond
	toastDuration = 4 * time.Second
	toastVisible  = 3
)

// Build returns the app and the viewport that shows the toasts.
func (t Toaster) Build() tela.Node {
	st := &UseStyle().Toast
	_, bump := tela.UseState(0)
	store := tela.UseOnce(func() *toastStore { return &toastStore{bump: bump} })
	hovered, setHovered := tela.UseState(false)

	dur := t.Duration
	if dur == 0 {
		dur = toastDuration
	}
	store.setConfig(toastConfig{duration: dur})
	toasts, heights := store.snapshot()

	pos := t.Position
	if pos == "" {
		pos = ToastBottomRight
	}
	posClass := pick("Toaster", "Position", st.Positions, pos)
	y, _, _ := strings.Cut(string(pos), "-")
	if y != "top" {
		y = "bottom"
	}
	lift := float32(-1)
	if y == "top" {
		lift = 1
	}
	visible := t.VisibleToasts
	if visible <= 0 {
		visible = toastVisible
	}
	expanded := (t.Expand || hovered) && len(toasts) > 0
	tela.UseEffectDo(func() {
		if len(toasts) == 0 && hovered {
			setHovered.Set(false)
		}
	}, len(toasts), hovered)

	icons := t.Icons
	closeIcon := t.CloseIcon
	if closeIcon == (tela.Icon{}) {
		closeIcon = toastX
	}

	items := make([]tela.Child, 0, len(toasts))
	offset := float32(0)
	var frontH float32
	if len(toasts) > 0 {
		frontH = heights[toasts[0].id]
	}
	for i, td := range toasts {
		own := heights[td.id]
		var cls []string
		cls = append(cls, st.Item, st.Sides[y], st.Enter[y], t.ToastClass)
		collapsedBehind := !expanded && i > 0
		if i >= visible {
			cls = append(cls, st.Hidden)
		}
		cls = append(cls, fmt.Sprintf("z-[%d]", len(toasts)-i))
		switch {
		case expanded:
			if offset != 0 {
				cls = append(cls, fmt.Sprintf("translate-y-[%spx]", fmtPx(lift*offset)))
			}
			if own > 0 {
				cls = append(cls, fmt.Sprintf("h-[%spx]", fmtPx(own)))
			}
		case i > 0:
			cls = append(cls, fmt.Sprintf("translate-y-[%spx]", fmtPx(lift*toastGap*float32(i))),
				fmt.Sprintf("scale-[%s]", fmtPx(1-0.05*float32(i))))
			// A toast not measured yet has its natural height, which is what
			// toastItem measures: a forced one would be measured back.
			if frontH > 0 && own > 0 {
				cls = append(cls, fmt.Sprintf("h-[%spx]", fmtPx(frontH)))
			}
		default:
			if own > 0 {
				cls = append(cls, fmt.Sprintf("h-[%spx]", fmtPx(own)))
			}
		}
		offset += own + toastGap
		items = append(items, tela.WithKey(toastItem{
			store: store, data: td, class: joinClass(cls...), exit: st.Exit[y],
			collapsedBehind: collapsedBehind, expanded: expanded, paused: hovered, unforced: own == 0,
			closeButton: t.CloseButton, icons: icons, closeIcon: closeIcon,
		}, td.id))
	}

	root := joinClass(st.Root, when(t.Inline, st.Inline), when(!t.Inline, st.Fixed), posClass, t.Class)
	viewport := h.Div(cls(root), h.Role("region"), h.AriaLabel("Notifications"), t.Props,
		h.OnMouseEnter(func() { setHovered.Set(true) }), h.OnMouseLeave(func() { setHovered.Set(false) }),
		h.Presence(items...))
	var layer tela.Child = viewport
	if !t.Inline {
		layer = h.Portal(viewport)
	}
	if t.Children == nil {
		return h.Fragment(layer)
	}
	return h.Fragment(toastCtx.Provider(store, t.Children), layer)
}

// fmtPx formats a number for a class: shortest, no exponent.
func fmtPx(v float32) string {
	s := fmt.Sprintf("%.3f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

// toastItem is one toast: its card, its content, its timer and the
// measurement of its height.
type toastItem struct {
	store           *toastStore
	data            toastData
	class, exit     string
	collapsedBehind bool
	expanded        bool
	paused          bool
	unforced        bool // no height class applies yet: the root has its natural height
	closeButton     bool
	icons           ToastIcons
	closeIcon       tela.Icon
}

type toastTimer struct {
	gen  int
	left time.Duration
}

func (it toastItem) Build() tela.Node {
	st := &UseStyle().Toast
	d := it.data
	// The toast's height is what its root lays out: the body plus the
	// padding and border the recipe gives it. The root is measured while no
	// height class applies (a forced height would be measured back); the
	// difference to the body is kept, and a body that changes later (a
	// replaced toast, a narrower window) is that difference taller.
	body, root := tela.UseElementRef(), tela.UseElementRef()
	size, rootSize := tela.UseSize(body), tela.UseSize(root)
	ch, rh := size.Get().Height, rootSize.Get().Height
	chrome := tela.UseRef(float32(-1))
	tela.UseEffectDo(func() {
		switch {
		case ch <= 0:
		case it.unforced && rh > 0:
			chrome.Current = rh - ch
			it.store.setHeight(d.id, rh)
		case chrome.Current >= 0:
			it.store.setHeight(d.id, ch+chrome.Current)
		}
	}, ch, rh, it.unforced)

	// The timer counts the frame clock in ticks, so that pausing it keeps
	// what is left; a replaced toast starts over.
	cfg := it.store.config()
	life := d.duration
	if life == 0 {
		life = cfg.duration
	}
	if d.kind == toastLoading && d.duration == 0 {
		life = ToastForever
	}
	timer := tela.UseRef(toastTimer{gen: -1})
	if timer.Current.gen != d.gen {
		timer.Current = toastTimer{gen: d.gen, left: life}
	}
	tick := time.Duration(0)
	if life > 0 && !it.paused {
		tick = toastTick
	}
	tela.UseInterval(tick, func() {
		timer.Current.left -= toastTick
		if timer.Current.left <= 0 {
			it.store.dismiss(d.id, true)
		}
	})

	var ic tela.Icon
	spinner := ""
	switch d.kind {
	case toastSuccess:
		ic = firstIcon(it.icons.Success, toastCircleCheck)
	case toastError:
		ic = firstIcon(it.icons.Error, toastOctagonX)
	case toastInfo2:
		ic = firstIcon(it.icons.Info, toastInfo)
	case toastWarning:
		ic = firstIcon(it.icons.Warning, toastTriangleWarn)
	case toastLoading:
		ic, spinner = firstIcon(it.icons.Loading, toastLoader), st.Spinner
	}
	var iconBox tela.Child
	if ic != (tela.Icon{}) {
		iconBox = h.Div(cls(st.Icon), h.Icon(ic, cls(st.IconSvg, spinner)))
	}
	var desc, action tela.Child
	if d.description != "" {
		desc = h.Div(cls(st.Description), h.Text(d.description))
	}
	if d.action != "" {
		on := d.onAction
		action = h.Button(h.Type("button"), cls(st.Action), h.OnClick(func() {
			if on != nil {
				on()
			}
			it.store.dismiss(d.id, false)
		}), h.Text(d.action))
	}
	var closeBtn tela.Child
	if it.closeButton {
		closeBtn = h.Button(h.Type("button"), h.AriaLabel("Close toast"), cls(st.Close),
			h.OnClick(func() { it.store.dismiss(d.id, false) }), h.Icon(it.closeIcon, cls(st.CloseIcon)))
	}
	var bridge tela.Child
	if it.expanded {
		bridge = h.Div(cls(st.Bridge))
	}
	role := "status"
	if d.kind == toastError {
		role = "alert"
	}
	return h.Div(h.Ref(root), cls(it.class), h.Role(role), h.TabIndex(0), h.Exit(it.exit), closeBtn, bridge,
		h.Div(h.Ref(body), cls(st.Body, when(it.collapsedBehind, st.Collapsed)),
			iconBox,
			h.Div(cls(st.Content), h.Div(cls(st.Title), h.Text(d.title)), desc),
			action))
}

func firstIcon(a, b tela.Icon) tela.Icon {
	if a != (tela.Icon{}) {
		return a
	}
	return b
}
