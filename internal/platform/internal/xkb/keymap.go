package xkb

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// Real modifier indices and masks (the first eight modifier indices of
// every keymap).
const (
	modShift   = 0
	modLock    = 1
	modControl = 2
	numReal    = 8
	realMask   = 0xff

	maxKeycode = 0xffff // larger keycodes are rejected (evdev codes end well below)
	maxGroups  = 4      // format v1
	maxLevels  = 64
)

var realModNames = [numReal]string{"Shift", "Lock", "Control", "Mod1", "Mod2", "Mod3", "Mod4", "Mod5"}

// Keymap is a compiled keymap. It is immutable after ParseKeymap and safe
// to share; per-keyboard state lives in [State].
type Keymap struct {
	minKey uint32
	keys   []key // indexed by keycode - minKey

	modNames []string // modifier index → name: the real ones, then the virtual ones in declaration order
	mapping  []uint32 // modifier index → resolved mask (real bits, or a virtual modifier's own bit when it has an explicit canonical mapping)

	numGroups int

	// Masks of the modifiers reported in platform.Modifiers, and of the
	// ones the lookup needs.
	shiftMask, lockMask, ctrlMask, altMask, superMask uint32
}

type rangePolicy uint8

const (
	rangeWrap rangePolicy = iota
	rangeClamp
	rangeRedirect
)

type key struct {
	exists   bool
	name     string
	groups   []keyGroup
	policy   rangePolicy
	redirect int
	repeats  bool
	modmap   uint32 // real modifiers of modifier_map
	vmodmap  uint32 // virtual modifiers (index-space bits) this key binds

	explicitRepeat  bool
	explicitVMods   bool
	explicitActions bool
}

type keyGroup struct {
	typ    *keyType
	levels []level
}

// level is one level of a key group, with its lookup results precomputed
// so that a lookup never allocates.
type level struct {
	syms []Keysym // the keysyms of the level (usually one; none for NoSymbol)
	one  Keysym   // syms[0] if there is exactly one keysym, else NoSymbol

	key  platform.Key
	text string

	// The same after the Caps Lock transformation (uppercase).
	capsOne  Keysym
	capsKey  platform.Key
	capsText string
}

type keyType struct {
	name    string
	modsRaw uint32 // in modifier-index space (virtual modifiers as their own bits)
	mods    uint32 // resolved to real modifiers
	levels  int
	entries []typeEntry
}

type typeEntry struct {
	modsRaw, preserveRaw uint32
	mods, preserve       uint32
	level                int
	active               bool // its modifiers are not all unmapped virtual modifiers
}

// interp is a symbol interpretation of the compat section, reduced to the
// fields a client needs.
type interp struct {
	sym          Keysym // NoSymbol matches any keysym
	match        matchOp
	mods         uint32 // real modifiers
	vmod         int    // virtual modifier index, or -1
	repeat       bool
	levelOneOnly bool
}

type matchOp uint8

const (
	matchNone matchOp = iota
	matchAnyOrNone
	matchAny
	matchAll
	matchExactly
)

// defaultInterp applies to keys no interpretation matches: they repeat
// (libxkbcommon's default_interpret).
var defaultInterp = interp{repeat: true, vmod: -1, match: matchAnyOrNone}

// keyAt returns the key of keycode kc, or nil.
func (km *Keymap) keyAt(kc uint32) *key {
	if kc < km.minKey || kc-km.minKey >= uint32(len(km.keys)) {
		return nil
	}
	k := &km.keys[kc-km.minKey]
	if !k.exists {
		return nil
	}
	return k
}

// Repeats reports whether keycode kc (an XKB keycode: evdev code + 8)
// repeats when held. Modifier keys usually do not.
func (km *Keymap) Repeats(kc uint32) bool {
	k := km.keyAt(kc)
	return k != nil && k.repeats
}

// NumGroups returns the number of layouts (groups) of the keymap.
func (km *Keymap) NumGroups() int { return km.numGroups }

// ModMask returns the resolved mask of a modifier by name (a real
// modifier such as "Shift" or "Mod1", or a virtual one such as "NumLock"
// or "LevelThree"), and false if the keymap has no such modifier.
func (km *Keymap) ModMask(name string) (uint32, bool) {
	i := km.modIndex(name)
	if i < 0 {
		return 0, false
	}
	return km.mapping[i], true
}

func (km *Keymap) modIndex(name string) int {
	for i, n := range km.modNames {
		if i < numReal && strings.EqualFold(n, name) || n == name {
			return i
		}
	}
	return -1
}

// resolve maps a modifier-index-space mask to real modifiers (plus the own
// bits of virtual modifiers with a canonical mapping).
func (km *Keymap) resolve(raw uint32) uint32 {
	m := raw & realMask
	for i := numReal; i < len(km.mapping) && i < 32; i++ {
		if raw&(1<<i) != 0 {
			m |= km.mapping[i]
		}
	}
	return m
}

// --- compilation ---

type compiler struct {
	km *Keymap

	keycodes map[string]uint32
	aliases  map[string]string

	vmodExplicit map[int]uint32
	types        []*keyType
	typeByName   map[string]*keyType

	interps  []interp
	defaults interpDefaults

	keyDefaults keyDefaults

	modmaps []modmapEntry
}

type modmapEntry struct {
	mod  int
	key  string // key name, or ""
	sym  Keysym
	line int
}

// ParseKeymap parses and compiles a keymap in the xkb_v1 text format, as
// sent by a Wayland compositor (wl_keyboard.keymap with format xkb_v1).
// The text may end with NUL bytes.
func ParseKeymap(text []byte) (*Keymap, error) {
	if i := indexNUL(text); i >= 0 {
		text = text[:i]
	}
	secs, err := parseKeymap(text)
	if err != nil {
		return nil, err
	}
	c := &compiler{
		km:           &Keymap{modNames: realModNames[:]},
		keycodes:     map[string]uint32{},
		aliases:      map[string]string{},
		vmodExplicit: map[int]uint32{},
		typeByName:   map[string]*keyType{},
		defaults:     interpDefaults{vmod: -1},
	}
	c.km.modNames = append([]string(nil), realModNames[:]...)
	have := map[string]bool{}
	// Sections are compiled in dependency order: keycodes, types,
	// compat, symbols.
	for _, kind := range []string{"keycodes", "types", "compat", "symbols"} {
		for _, s := range secs {
			if s.kind != kind {
				continue
			}
			if have[kind] {
				return nil, fmt.Errorf("xkb: the keymap has two xkb_%s sections", kind)
			}
			have[kind] = true
			if kind == "symbols" {
				c.sizeKeys()
			}
			if err := c.section(s); err != nil {
				return nil, err
			}
		}
	}
	for _, kind := range []string{"keycodes", "types", "symbols"} {
		if !have[kind] {
			return nil, fmt.Errorf("xkb: the keymap has no xkb_%s section", kind)
		}
	}
	if err := c.finish(); err != nil {
		return nil, err
	}
	return c.km, nil
}

func indexNUL(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}

func (c *compiler) section(s section) error {
	for i := range s.stmts {
		st := &s.stmts[i]
		var err error
		switch s.kind {
		case "keycodes":
			err = c.keycodeStmt(st)
		case "types":
			err = c.typesStmt(st)
		case "compat":
			err = c.compatStmt(st)
		case "symbols":
			err = c.symbolsStmt(st)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func errAt(line int, format string, args ...any) error {
	return fmt.Errorf("xkb: line %d: %s", line, fmt.Sprintf(format, args...))
}

// --- keycodes ---

func (c *compiler) keycodeStmt(st *stmt) error {
	switch st.kind {
	case sKeycode:
		if st.rhs.kind != eNum || st.rhs.num < 0 || st.rhs.num > maxKeycode {
			return errAt(st.line, "keycode of <%s> must be a number between 0 and %d", st.name, maxKeycode)
		}
		c.keycodes[st.name] = uint32(st.rhs.num)
	case sAlias:
		c.aliases[st.name] = st.rhs.name
	case sVMods:
		return c.declareVMods(st)
	}
	// minimum, maximum and indicator names do not matter.
	return nil
}

// keycode resolves a key name through the aliases.
func (c *compiler) keycode(name string) (uint32, bool) {
	for range 8 {
		if kc, ok := c.keycodes[name]; ok {
			return kc, true
		}
		real, ok := c.aliases[name]
		if !ok {
			return 0, false
		}
		name = real
	}
	return 0, false
}

// --- modifiers ---

func (c *compiler) declareVMods(st *stmt) error {
	for _, e := range st.list {
		nameExpr, val := e, (*expr)(nil)
		if e.kind == eAssign {
			nameExpr, val = e.lhs, e.rhs
		}
		name := nameExpr.name
		idx := c.km.modIndex(name)
		if idx >= 0 && idx < numReal {
			return errAt(st.line, "%q is a real modifier, not a virtual one", name)
		}
		if idx < 0 {
			if len(c.km.modNames) >= 32 {
				return errAt(st.line, "too many virtual modifiers (at most %d)", 32-numReal)
			}
			idx = len(c.km.modNames)
			c.km.modNames = append(c.km.modNames, name)
		}
		if val != nil {
			m, err := c.mask(val)
			if err != nil {
				return err
			}
			c.vmodExplicit[idx] = m
		}
	}
	return nil
}

// mask evaluates a modifier mask expression into modifier-index space.
func (c *compiler) mask(e *expr) (uint32, error) {
	switch e.kind {
	case eNum:
		return uint32(e.num), nil
	case eIdent:
		if e.index != nil || strings.Contains(e.name, ".") {
			break
		}
		switch strings.ToLower(e.name) {
		case "none":
			return 0, nil
		case "all":
			return realMask, nil
		}
		i := c.km.modIndex(e.name)
		if i < 0 {
			return 0, errAt(e.line, "unknown modifier %q", e.name)
		}
		return 1 << i, nil
	case eBinary:
		l, err := c.mask(e.lhs)
		if err != nil {
			return 0, err
		}
		r, err := c.mask(e.rhs)
		if err != nil {
			return 0, err
		}
		switch e.op {
		case '+':
			return l | r, nil
		case '-':
			return l &^ r, nil
		}
	case eUnary:
		if e.op == '+' {
			return c.mask(e.rhs)
		}
	}
	return 0, errAt(e.line, "malformed modifier mask")
}

// realModIndex returns the index of a real modifier name.
func realModIndex(name string) int {
	for i, n := range realModNames {
		if strings.EqualFold(n, name) {
			return i
		}
	}
	return -1
}

// --- types ---

func (c *compiler) typesStmt(st *stmt) error {
	switch st.kind {
	case sVMods:
		return c.declareVMods(st)
	case sType:
		return c.keyType(st)
	}
	return nil // type.* defaults have no meaning in a resolved keymap
}

func (c *compiler) keyType(st *stmt) error {
	t := &keyType{name: st.name, levels: 1}
	entry := func(mods uint32) *typeEntry {
		for i := range t.entries {
			if t.entries[i].modsRaw == mods {
				return &t.entries[i]
			}
		}
		t.entries = append(t.entries, typeEntry{modsRaw: mods})
		return &t.entries[len(t.entries)-1]
	}
	for i := range st.body {
		b := &st.body[i]
		if b.kind != sVar || b.lhs == nil || b.rhs == nil {
			continue
		}
		switch strings.ToLower(b.lhs.name) {
		case "modifiers":
			m, err := c.mask(b.rhs)
			if err != nil {
				return err
			}
			t.modsRaw = m
		case "map":
			if b.lhs.index == nil {
				return errAt(b.line, "map without a modifier index in type %q", t.name)
			}
			m, err := c.mask(b.lhs.index)
			if err != nil {
				return err
			}
			lv, err := levelIndex(b.rhs)
			if err != nil {
				return err
			}
			e := entry(m)
			e.level = lv
			t.levels = max(t.levels, lv+1)
		case "preserve":
			if b.lhs.index == nil {
				return errAt(b.line, "preserve without a modifier index in type %q", t.name)
			}
			m, err := c.mask(b.lhs.index)
			if err != nil {
				return err
			}
			p, err := c.mask(b.rhs)
			if err != nil {
				return err
			}
			entry(m).preserveRaw = p
		case "level_name", "levelname":
			if b.lhs.index != nil {
				lv, err := levelIndex(b.lhs.index)
				if err != nil {
					return err
				}
				t.levels = max(t.levels, lv+1)
			}
		}
	}
	// Entries only matter for modifiers the type uses.
	for i := range t.entries {
		t.entries[i].modsRaw &= t.modsRaw
	}
	if _, dup := c.typeByName[t.name]; !dup {
		c.types = append(c.types, t)
	}
	c.typeByName[t.name] = t
	return nil
}

// levelIndex evaluates a shift level (1-based number or LevelN) to a
// 0-based index.
func levelIndex(e *expr) (int, error) {
	var n int64
	switch e.kind {
	case eNum:
		n = e.num
	case eIdent:
		s := strings.ToLower(e.name)
		if !strings.HasPrefix(s, "level") {
			return 0, errAt(e.line, "malformed shift level %q", e.name)
		}
		if _, err := fmt.Sscanf(s[5:], "%d", &n); err != nil {
			return 0, errAt(e.line, "malformed shift level %q", e.name)
		}
	default:
		return 0, errAt(e.line, "malformed shift level")
	}
	if n < 1 || n > maxLevels {
		return 0, errAt(e.line, "shift level %d out of range", n)
	}
	return int(n - 1), nil
}

// groupIndex evaluates a group index (1-based number or GroupN) to a
// 0-based index.
func groupIndex(e *expr) (int, error) {
	var n int64
	switch e.kind {
	case eNum:
		n = e.num
	case eIdent:
		s := strings.ToLower(e.name)
		if !strings.HasPrefix(s, "group") {
			return 0, errAt(e.line, "malformed group %q", e.name)
		}
		if _, err := fmt.Sscanf(s[5:], "%d", &n); err != nil {
			return 0, errAt(e.line, "malformed group %q", e.name)
		}
	default:
		return 0, errAt(e.line, "malformed group index")
	}
	if n < 1 || n > maxGroups {
		return 0, errAt(e.line, "group %d out of range (format v1 allows %d)", n, maxGroups)
	}
	return int(n - 1), nil
}

// boolValue evaluates a boolean field value.
func boolValue(e *expr) (bool, error) {
	if e == nil {
		return true, nil
	}
	switch e.kind {
	case eNum:
		return e.num != 0, nil
	case eIdent:
		switch strings.ToLower(e.name) {
		case "true", "yes", "on":
			return true, nil
		case "false", "no", "off":
			return false, nil
		}
	}
	return false, errAt(e.line, "malformed boolean")
}

// --- compat ---

type interpDefaults struct {
	repeat       bool
	levelOneOnly bool
	vmod         int
}

func (c *compiler) compatStmt(st *stmt) error {
	switch st.kind {
	case sVMods:
		return c.declareVMods(st)
	case sInterp:
		return c.interpret(st)
	case sVar:
		// interpret.repeat = False; and similar defaults.
		name := strings.ToLower(st.lhs.name)
		if !strings.HasPrefix(name, "interpret.") {
			return nil
		}
		v := st.rhs
		if st.neg {
			v = &expr{kind: eIdent, name: "false", line: st.line}
		}
		return c.interpField(&c.defaults, strings.TrimPrefix(name, "interpret."), v)
	}
	return nil
}

func (c *compiler) interpField(d *interpDefaults, field string, v *expr) error {
	switch field {
	case "repeat":
		b, err := boolValue(v)
		if err != nil {
			return err
		}
		d.repeat = b
	case "usemodmapmods", "usemodmap":
		if v == nil || v.kind != eIdent {
			return errAt(0, "malformed useModMapMods")
		}
		switch strings.ToLower(v.name) {
		case "level1", "levelone":
			d.levelOneOnly = true
		case "anylevel", "any":
			d.levelOneOnly = false
		default:
			return errAt(v.line, "malformed useModMapMods %q", v.name)
		}
	case "virtualmodifier", "virtualmod":
		if v == nil || v.kind != eIdent {
			return errAt(0, "malformed virtualModifier")
		}
		i := c.km.modIndex(v.name)
		if i < numReal {
			return errAt(v.line, "unknown virtual modifier %q", v.name)
		}
		d.vmod = i
	}
	// action, locking: the compositor runs actions.
	return nil
}

func (c *compiler) interpret(st *stmt) error {
	in := interp{match: matchAnyOrNone, mods: realMask}
	head := st.rhs
	symExpr := head
	if head.kind == eBinary && head.op == '+' {
		symExpr = head.lhs
		pred := head.rhs
		if pred.kind == eCall {
			switch strings.ToLower(pred.name) {
			case "noneof":
				in.match = matchNone
			case "anyofornone":
				in.match = matchAnyOrNone
			case "anyof":
				in.match = matchAny
			case "allof":
				in.match = matchAll
			case "exactly":
				in.match = matchExactly
			default:
				return errAt(pred.line, "unknown interpret predicate %q", pred.name)
			}
			in.mods = 0
			if len(pred.args) > 1 {
				return errAt(pred.line, "interpret predicate takes one argument")
			}
			if len(pred.args) == 1 {
				m, err := c.mask(pred.args[0])
				if err != nil {
					return err
				}
				in.mods = m & realMask
			}
		} else {
			m, err := c.mask(pred)
			if err != nil {
				return err
			}
			in.match, in.mods = matchExactly, m&realMask
		}
	}
	if symExpr.kind == eIdent && strings.EqualFold(symExpr.name, "any") {
		in.sym = NoSymbol
	} else {
		ks, err := c.keysym(symExpr)
		if err != nil {
			return err
		}
		in.sym = ks
	}
	d := c.defaults
	for i := range st.body {
		b := &st.body[i]
		if b.kind != sVar || b.lhs == nil {
			continue
		}
		v := b.rhs
		if b.neg {
			v = &expr{kind: eIdent, name: "false", line: b.line}
		}
		if err := c.interpField(&d, strings.ToLower(b.lhs.name), v); err != nil {
			return err
		}
	}
	in.repeat, in.levelOneOnly, in.vmod = d.repeat, d.levelOneOnly, d.vmod
	c.interps = append(c.interps, in)
	return nil
}

// keysym evaluates a keysym in a symbol list or an interpretation.
func (c *compiler) keysym(e *expr) (Keysym, error) {
	switch e.kind {
	case eNum:
		if e.num < 0 || e.num > int64(keysymMax) {
			return NoSymbol, errAt(e.line, "keysym %d out of range", e.num)
		}
		if e.num < 10 {
			return Keysym('0' + e.num), nil // the digits are keysym names
		}
		return Keysym(e.num), nil
	case eIdent:
		if e.index != nil || strings.Contains(e.name, ".") {
			break
		}
		switch strings.ToLower(e.name) {
		case "any", "nosymbol":
			return NoSymbol, nil
		case "none", "voidsymbol":
			return VoidSymbol, nil
		}
		if ks, ok := KeysymFromName(e.name); ok {
			return ks, nil
		}
		// libxkbcommon warns and uses NoSymbol; so does the compositor
		// that produced this keymap.
		return NoSymbol, nil
	}
	return NoSymbol, errAt(e.line, "malformed keysym")
}

// --- symbols ---

// keyDefaults are the key.* defaults of the symbols section.
type keyDefaults struct {
	typeName string
	repeat   *bool
}

func (c *compiler) symbolsStmt(st *stmt) error {
	switch st.kind {
	case sVMods:
		return c.declareVMods(st)
	case sKey:
		return c.key(st)
	case sModMap:
		mod := realModIndex(st.lhs.name)
		if mod < 0 {
			if strings.EqualFold(st.lhs.name, "none") {
				return nil
			}
			return errAt(st.line, "modifier_map needs a real modifier, not %q", st.lhs.name)
		}
		for _, e := range st.list {
			if e.kind == eKeyName {
				c.modmaps = append(c.modmaps, modmapEntry{mod: mod, key: e.name, line: e.line})
				continue
			}
			ks, err := c.keysym(e)
			if err != nil {
				return err
			}
			c.modmaps = append(c.modmaps, modmapEntry{mod: mod, sym: ks, line: e.line})
		}
	case sVar:
		name := strings.ToLower(st.lhs.name)
		switch name {
		case "key.type":
			if st.rhs == nil || st.rhs.kind != eString {
				return errAt(st.line, "key.type must be a string")
			}
			c.keyDefaults.typeName = st.rhs.name
		case "key.repeat", "key.repeats", "key.repeating":
			b, err := boolValue(st.rhs)
			if err != nil {
				return err
			}
			if st.neg {
				b = false
			}
			c.keyDefaults.repeat = &b
		}
		// name[N] (group names) and other defaults do not matter.
	}
	return nil
}

// symbolsKey is a key of the symbols section before type resolution.
type symbolsKey struct {
	syms      [maxGroups][][]Keysym // per group, per level
	hasGroup  [maxGroups]bool
	typeName  [maxGroups]string
	allType   string
	nextGroup int
}

func (c *compiler) key(st *stmt) error {
	kc, ok := c.keycode(st.name)
	if !ok {
		// libxkbcommon skips symbols for keys the keycodes do not
		// define.
		return nil
	}
	sk := &symbolsKey{}
	k := key{name: st.name}
	if c.keyDefaults.repeat != nil {
		k.repeats, k.explicitRepeat = *c.keyDefaults.repeat, true
	}
	for i := range st.body {
		b := &st.body[i]
		if b.lhs == nil {
			// A bare symbol list: the next group.
			if sk.nextGroup >= maxGroups {
				return errAt(b.line, "key <%s> has more than %d groups", st.name, maxGroups)
			}
			if err := c.symbolList(sk, sk.nextGroup, b.rhs, st.name); err != nil {
				return err
			}
			sk.nextGroup++
			continue
		}
		field := strings.ToLower(b.lhs.name)
		v := b.rhs
		switch field {
		case "type":
			if v == nil || v.kind != eString {
				return errAt(b.line, "type of key <%s> must be a string", st.name)
			}
			if b.lhs.index == nil {
				sk.allType = v.name
				continue
			}
			g, err := groupIndex(b.lhs.index)
			if err != nil {
				return err
			}
			sk.typeName[g] = v.name
		case "symbols":
			g := sk.nextGroup
			if b.lhs.index != nil {
				var err error
				if g, err = groupIndex(b.lhs.index); err != nil {
					return err
				}
			}
			if g >= maxGroups {
				return errAt(b.line, "key <%s> has more than %d groups", st.name, maxGroups)
			}
			if err := c.symbolList(sk, g, v, st.name); err != nil {
				return err
			}
			sk.nextGroup = max(sk.nextGroup, g+1)
		case "actions":
			k.explicitActions = true
		case "repeat", "repeats", "repeating":
			r, err := boolValue(v)
			if err != nil {
				return err
			}
			if b.neg {
				r = false
			}
			k.repeats, k.explicitRepeat = r, true
		case "virtualmods", "virtualmodifiers", "vmods":
			m, err := c.mask(v)
			if err != nil {
				return err
			}
			k.vmodmap, k.explicitVMods = m&^realMask, true
		case "groupswrap", "wrapgroups":
			if on, err := boolValue(v); err != nil {
				return err
			} else if on != b.neg {
				k.policy = rangeWrap
			} else {
				k.policy = rangeClamp
			}
		case "groupsclamp", "clampgroups":
			if on, err := boolValue(v); err != nil {
				return err
			} else if on != b.neg {
				k.policy = rangeClamp
			} else {
				k.policy = rangeWrap
			}
		case "groupsredirect", "redirectgroups":
			g, err := groupIndex(v)
			if err != nil {
				return err
			}
			k.policy, k.redirect = rangeRedirect, g
		case "locking", "locks", "lock", "radiogroup", "permanentradiogroup", "allownone",
			"overlay", "overlay1", "overlay2", "behavior", "behaviour":
			// Key behaviors are run by the compositor.
		default:
			return errAt(b.line, "unknown field %q in key <%s>: the keymap uses a feature this parser does not know, and ignoring it could change what the key types", b.lhs.name, st.name)
		}
	}
	// Resolve the groups now that the whole key is known.
	n := 0
	for g := range maxGroups {
		if sk.hasGroup[g] {
			n = g + 1
		}
	}
	k.exists = true
	k.groups = make([]keyGroup, n)
	for g := range n {
		name := sk.typeName[g]
		if name == "" {
			name = sk.allType
		}
		if name == "" {
			name = c.keyDefaults.typeName
		}
		var t *keyType
		if name != "" {
			t = c.typeByName[name]
		} else {
			t = c.typeByName[automaticType(sk.syms[g])]
		}
		if t == nil {
			// libxkbcommon falls back to the first type (usually
			// ONE_LEVEL) when a type is missing.
			if len(c.types) == 0 {
				return errAt(st.line, "key <%s> needs a key type, but the keymap defines none", st.name)
			}
			t = c.types[0]
		}
		levels := make([]level, t.levels)
		for l := range min(len(levels), len(sk.syms[g])) {
			levels[l].syms = sk.syms[g][l]
		}
		k.groups[g] = keyGroup{typ: t, levels: levels}
	}
	c.growKeys(kc)
	// A second definition of the same key replaces the first (a resolved
	// keymap has none).
	c.km.keys[kc-c.km.minKey] = k
	return nil
}

// growKeys makes room for keycode kc in the key table. The table is
// sized from the keycodes section first (sizeKeys), so this only grows
// for keycodes defined later, which a resolved keymap does not have.
func (c *compiler) growKeys(kc uint32) {
	km := c.km
	if len(km.keys) == 0 {
		km.minKey = kc
		km.keys = make([]key, 1)
		return
	}
	if kc < km.minKey {
		grown := make([]key, int(km.minKey-kc)+len(km.keys))
		copy(grown[km.minKey-kc:], km.keys)
		km.keys, km.minKey = grown, kc
		return
	}
	if need := int(kc-km.minKey) + 1; need > len(km.keys) {
		km.keys = append(km.keys, make([]key, need-len(km.keys))...)
	}
}

// sizeKeys allocates the key table for the keycodes of the keycodes
// section.
func (c *compiler) sizeKeys() {
	if len(c.keycodes) == 0 || len(c.km.keys) > 0 {
		return
	}
	lo, hi := uint32(maxKeycode), uint32(0)
	for _, kc := range c.keycodes {
		lo, hi = min(lo, kc), max(hi, kc)
	}
	c.km.minKey = lo
	c.km.keys = make([]key, hi-lo+1)
}

func (c *compiler) symbolList(sk *symbolsKey, g int, v *expr, keyName string) error {
	if v == nil || v.kind != eList {
		return errAt(0, "symbols of key <%s> must be a list", keyName)
	}
	levels := make([][]Keysym, len(v.args))
	for i, a := range v.args {
		var elems []*expr
		if a.kind == eBraces {
			elems = a.args
		} else {
			elems = []*expr{a}
		}
		for _, e := range elems {
			ks, err := c.keysym(e)
			if err != nil {
				return err
			}
			if ks != NoSymbol {
				levels[i] = append(levels[i], ks)
			}
		}
	}
	sk.syms[g] = levels
	sk.hasGroup[g] = true
	return nil
}

// automaticType chooses the type of a group without one, like
// libxkbcommon's FindAutomaticType.
func automaticType(levels [][]Keysym) string {
	sym := func(i int) Keysym {
		if i >= len(levels) || len(levels[i]) == 0 {
			return NoSymbol
		}
		return levels[i][0]
	}
	w := len(levels)
	switch {
	case w <= 1:
		return "ONE_LEVEL"
	case w == 2:
		s0, s1 := sym(0), sym(1)
		switch {
		case s0.isLower() && s1.isUpperOrTitle():
			return "ALPHABETIC"
		case s0.IsKeypad() || s1.IsKeypad():
			return "KEYPAD"
		}
		return "TWO_LEVEL"
	case w <= 4:
		s0, s1, s2, s3 := sym(0), sym(1), sym(2), NoSymbol
		if w == 4 {
			s3 = sym(3)
		}
		switch {
		case s0.isLower() && s1.isUpperOrTitle():
			if s2.isLower() && s3.isUpperOrTitle() {
				return "FOUR_LEVEL_ALPHABETIC"
			}
			return "FOUR_LEVEL_SEMIALPHABETIC"
		case s0.IsKeypad() || s1.IsKeypad():
			return "FOUR_LEVEL_KEYPAD"
		}
		return "FOUR_LEVEL"
	}
	return "" // falls back to the first type
}

// --- derivation ---

func (c *compiler) finish() error {
	km := c.km
	// Modifier maps.
	for _, m := range c.modmaps {
		var k *key
		if m.key != "" {
			if kc, ok := c.keycode(m.key); ok {
				k = km.keyAt(kc)
			}
		} else {
			k = c.keyForSym(m.sym)
		}
		if k != nil {
			k.modmap |= 1 << m.mod
		}
	}
	// Interpretations, most specific first (libxkbcommon's CopyInterps
	// order): with a keysym before without; within each, Exactly, AllOf,
	// NoneOf, AnyOf, AnyOfOrNone.
	var sorted []interp
	for _, withSym := range []bool{true, false} {
		for _, m := range []matchOp{matchExactly, matchAll, matchNone, matchAny, matchAnyOrNone} {
			for _, in := range c.interps {
				if in.match == m && (in.sym != NoSymbol) == withSym {
					sorted = append(sorted, in)
				}
			}
		}
	}
	for i := range km.keys {
		k := &km.keys[i]
		if !k.exists {
			continue
		}
		applyInterps(k, sorted)
		km.numGroups = max(km.numGroups, len(k.groups))
	}
	// Virtual modifier mapping.
	km.mapping = make([]uint32, len(km.modNames))
	for i := range numReal {
		km.mapping[i] = 1 << i
	}
	for i, m := range c.vmodExplicit {
		km.mapping[i] = m
	}
	for i := range km.keys {
		k := &km.keys[i]
		for v := numReal; v < len(km.modNames); v++ {
			if k.vmodmap&(1<<v) != 0 {
				km.mapping[v] |= k.modmap
			}
		}
	}
	// Resolve the key types.
	for _, t := range c.types {
		t.mods = km.resolve(t.modsRaw)
		for i := range t.entries {
			e := &t.entries[i]
			e.mods = km.resolve(e.modsRaw)
			e.preserve = km.resolve(e.preserveRaw)
			e.active = e.modsRaw == 0 || e.mods != 0
		}
	}
	// Precompute every level's lookup results.
	cache := map[Keysym]*level{}
	for i := range km.keys {
		k := &km.keys[i]
		for g := range k.groups {
			for l := range k.groups[g].levels {
				precompute(&k.groups[g].levels[l], cache)
			}
		}
	}
	km.shiftMask = 1 << modShift
	km.lockMask = 1 << modLock
	km.ctrlMask = 1 << modControl
	notAlt := km.shiftMask | km.lockMask | km.ctrlMask
	for _, n := range []string{"LevelThree", "LevelFive", "NumLock"} {
		if m, ok := km.ModMask(n); ok {
			notAlt |= m
		}
	}
	km.altMask = km.vmodOr("Alt", 1<<3) &^ notAlt
	km.superMask = km.vmodOr("Super", 1<<6) &^ (notAlt | km.altMask)
	return nil
}

// vmodOr returns the mapping of a virtual modifier, or def if the keymap
// does not map it.
func (km *Keymap) vmodOr(name string, def uint32) uint32 {
	if m, ok := km.ModMask(name); ok && m != 0 {
		return m
	}
	return def
}

// keyForSym finds the key a modifier_map keysym entry names: the first
// key, by group, then level (the first two only), then keycode, whose
// level holds exactly that keysym (libxkbcommon's FindKeyForSymbol).
func (c *compiler) keyForSym(ks Keysym) *key {
	km := c.km
	for g := range maxGroups {
		for l := range 2 {
			for i := range km.keys {
				k := &km.keys[i]
				if !k.exists || g >= len(k.groups) || l >= len(k.groups[g].levels) {
					continue
				}
				if s := k.groups[g].levels[l].syms; len(s) == 1 && s[0] == ks {
					return k
				}
			}
		}
	}
	return nil
}

// applyInterps derives the repeat flag and the virtual modifier map of a
// key from the interpretations, like libxkbcommon's ApplyInterpsToKey.
func applyInterps(k *key, interps []interp) {
	if !k.explicitRepeat {
		k.repeats = false
	}
	if k.explicitActions {
		// Keys with explicit actions take nothing from interpretations
		// (and repeat only if they say so).
		return
	}
	var vmodmap uint32
	for g := range k.groups {
		for l := range k.groups[g].levels {
			in := findInterp(k, g, l, interps)
			if in == nil {
				continue
			}
			if g == 0 && l == 0 && !k.explicitRepeat && in.repeat {
				k.repeats = true
			}
			if (g == 0 && l == 0) || !in.levelOneOnly {
				if in.vmod >= 0 {
					vmodmap |= 1 << in.vmod
				}
			}
		}
	}
	if !k.explicitVMods {
		k.vmodmap = vmodmap
	}
}

func findInterp(k *key, g, l int, interps []interp) *interp {
	syms := k.groups[g].levels[l].syms
	if len(syms) == 0 {
		return nil
	}
	for i := range interps {
		in := &interps[i]
		if in.sym != NoSymbol && (len(syms) > 1 || in.sym != syms[0]) {
			continue
		}
		mods := k.modmap
		if in.levelOneOnly && l != 0 {
			mods = 0
		}
		var found bool
		switch in.match {
		case matchNone:
			found = in.mods&mods == 0
		case matchAnyOrNone:
			found = mods == 0 || in.mods&mods != 0
		case matchAny:
			found = in.mods&mods != 0
		case matchAll:
			found = in.mods&mods == in.mods
		case matchExactly:
			found = in.mods == mods
		}
		if found {
			return in
		}
	}
	return &defaultInterp
}

// precompute fills a level's lookup results, sharing strings between
// levels with the same keysym.
func precompute(lv *level, cache map[Keysym]*level) {
	if len(lv.syms) == 1 {
		ks := lv.syms[0]
		if c, ok := cache[ks]; ok {
			syms := lv.syms
			*lv = *c
			lv.syms = syms
			return
		}
		lv.one = ks
		lv.key, lv.text = keysymKey(ks), keysymText(ks)
		up := ks.ToUpper()
		lv.capsOne = up
		if up == ks {
			lv.capsKey, lv.capsText = lv.key, lv.text
		} else {
			lv.capsKey, lv.capsText = keysymKey(up), keysymText(up)
		}
		cp := *lv
		cache[ks] = &cp
		return
	}
	// Zero or several keysyms: no single keysym; the text is the
	// concatenation (xkb_state_key_get_utf8).
	var text, capsText strings.Builder
	for _, ks := range lv.syms {
		text.WriteString(keysymText(ks))
		capsText.WriteString(keysymText(ks.ToUpper()))
	}
	lv.text, lv.capsText = text.String(), capsText.String()
	lv.key, lv.capsKey = platform.KeyUnidentified, platform.KeyUnidentified
	if lv.text != "" {
		lv.key = platform.Key(lv.text)
	}
	if lv.capsText != "" {
		lv.capsKey = platform.Key(lv.capsText)
	}
}
