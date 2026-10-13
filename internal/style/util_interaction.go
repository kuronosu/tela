package style

func registerInteraction(r *registry) {
	cursors := map[string]Op{}
	for i, name := range cursorNames {
		c := Cursor(i)
		cursors[name] = op(func(e *env) { e.s.Cursor = c })
	}
	r.enum("cursor", false, cursors)

	r.enum("pointer-events", false, map[string]Op{
		"none": op(func(e *env) { e.s.PointerEvents = PointerEventsNone }),
		"auto": op(func(e *env) { e.s.PointerEvents = PointerEventsAuto }),
	})
	r.enum("select", false, map[string]Op{
		"none": op(func(e *env) { e.s.UserSelect = UserSelectNone }),
		"text": op(func(e *env) { e.s.UserSelect = UserSelectText }),
		"all":  op(func(e *env) { e.s.UserSelect = UserSelectAll }),
		"auto": op(func(e *env) { e.s.UserSelect = UserSelectAuto }),
	})

	// Scrolling. scroll-auto and scroll-smooth are Tailwind's
	// scroll-behavior; scroll-instant and scrollbar-* are Tela's own.
	r.enum("scroll", false, map[string]Op{
		"auto":    op(func(e *env) { e.s.ScrollBehavior = ScrollBehaviorAuto }),
		"smooth":  op(func(e *env) { e.s.ScrollBehavior = ScrollBehaviorSmooth }),
		"instant": op(func(e *env) { e.s.ScrollBehavior = ScrollBehaviorInstant }),
	})
	r.enum("scrollbar", false, map[string]Op{
		"classic": op(func(e *env) { e.s.Scrollbar = ScrollbarClassic }),
		"overlay": op(func(e *env) { e.s.Scrollbar = ScrollbarOverlay }),
	})
	for _, root := range []string{"overscroll", "overscroll-x", "overscroll-y"} {
		keys := map[string]Op{}
		for i, name := range overscrollNames {
			v := Overscroll(i)
			switch root {
			case "overscroll":
				keys[name] = op(func(e *env) { e.s.OverscrollX, e.s.OverscrollY = v, v })
			case "overscroll-x":
				keys[name] = op(func(e *env) { e.s.OverscrollX = v })
			default:
				keys[name] = op(func(e *env) { e.s.OverscrollY = v })
			}
		}
		r.enum(root, false, keys)
	}

	// Accepted and ignored: Tela always uses border-box and has no native
	// controls or compositor hints to configure.
	// Marker classes: they style nothing, but variants refer to them.
	r.static("group", Op{})
	r.static("peer", Op{})

	r.static("box-border", Op{})
	r.static("appearance-none", Op{})
	r.static("appearance-auto", Op{})
}

// registerUnsupported lists Tailwind features that Tela does not
// implement, with what to use instead (ARCHITECTURE section 10).
func registerUnsupported(r *registry) {
	const flexOrGrid = "floats are not supported; use flex or grid"
	for _, k := range []string{"float", "clear"} {
		r.unsupported[k] = flexOrGrid
	}
	r.unsupported["table"] = "tables are not supported; build the layout with grid (the kit Table component will be built on it)"
	r.unsupported["inline-table"] = r.unsupported["table"]
	r.unsupported["list-item"] = "list items are not supported; use regular nodes"
	r.unsupported["list"] = "list styling is not supported; use regular nodes"
	r.unsupported["box-content"] = "content-box is not supported: boxes are always border-box"
	r.unsupported["divide"] = "divide-* is not supported; put a border on the children or use gap"
	r.unsupported["space-x-reverse"] = "space-*-reverse is not supported; use flex-row-reverse or flex-col-reverse"
	r.unsupported["space-y-reverse"] = r.unsupported["space-x-reverse"]
	r.unsupported["columns"] = "multi-column layout is not supported; use grid"
	r.unsupported["break-before"] = "fragmentation is not supported"
	r.unsupported["break-after"] = r.unsupported["break-before"]
	r.unsupported["break-inside"] = r.unsupported["break-before"]
	r.unsupported["hyphens"] = "hyphenation is not supported yet"
	r.unsupported["indent"] = "text-indent is not supported yet; use padding"
	r.unsupported["align"] = "only align-baseline, align-top, align-middle, align-bottom, align-text-top and align-text-bottom are supported (on inline-block, inline-flex and inline images in text); use flex alignment (items-center) otherwise"
	r.unsupported["fill"] = "SVG fill and stroke are not supported yet; icons take the text color"
	r.unsupported["stroke"] = r.unsupported["fill"]
	r.unsupported["accent"] = "accent-color is not supported; style the control with classes"
	r.unsupported["caret"] = "caret-color is not supported yet"
	r.unsupported["perspective"] = "3D transforms are not supported; Tela transforms are 2D"
	r.unsupported["transform-3d"] = r.unsupported["perspective"]
	r.unsupported["backface"] = r.unsupported["perspective"]
	r.unsupported["rotate-x"] = r.unsupported["perspective"]
	r.unsupported["rotate-y"] = r.unsupported["perspective"]
	r.unsupported["rotate-z"] = r.unsupported["perspective"]
	r.unsupported["translate-z"] = r.unsupported["perspective"]
	r.unsupported["scale-z"] = r.unsupported["perspective"]
	r.unsupported["scroll"] = "scroll snapping and scroll margins are not supported yet"
	r.unsupported["snap"] = r.unsupported["scroll"]
	r.unsupported["touch"] = "touch-action is not supported yet"
	r.unsupported["resize"] = "resizable elements are not supported"
	r.unsupported["will-change"] = "will-change is not needed: Tela decides which boxes get a layer"
	r.unsupported["clip-path"] = "clip-path is not supported yet; use rounded-* and overflow-hidden"
	r.unsupported["mask"] = "masks are not supported"
	r.unsupported["forced-color-adjust"] = "forced colors are not supported"
	r.unsupported["field-sizing"] = "field-sizing is not supported"
	r.unsupported["object"] = "object-fit and object-position are not supported yet; size the Img explicitly"
	r.unsupported["contain"] = "CSS containment is not supported"
	r.unsupported["content-visibility"] = "content-visibility is not supported; use VirtualList"
	r.unsupported["decoration-clone"] = "box-decoration-break is not supported"
	r.unsupported["decoration-slice"] = r.unsupported["decoration-clone"]
	r.unsupported["border-spacing"] = "tables are not supported"
	r.unsupported["border-collapse"] = "tables are not supported"
}
