//go:build linux

package wire

import "fmt"

// ArgType is the wire type of a message argument.
type ArgType uint8

const (
	ArgInt    ArgType = iota // int32
	ArgUint                  // uint32
	ArgFixed                 // signed 24.8 fixed point
	ArgString                // length-prefixed, NUL-terminated, padded to 4 bytes
	ArgObject                // object ID, 0 when null
	ArgNewID                 // ID of an object created by this message
	ArgArray                 // length-prefixed bytes, padded to 4 bytes
	ArgFd                    // file descriptor sent out of band (SCM_RIGHTS)
)

func (t ArgType) String() string {
	switch t {
	case ArgInt:
		return "int"
	case ArgUint:
		return "uint"
	case ArgFixed:
		return "fixed"
	case ArgString:
		return "string"
	case ArgObject:
		return "object"
	case ArgNewID:
		return "new_id"
	case ArgArray:
		return "array"
	case ArgFd:
		return "fd"
	}
	return fmt.Sprintf("ArgType(%d)", uint8(t))
}

// Arg describes one argument of a request or event.
type Arg struct {
	Type     ArgType
	Nullable bool   // the string or object may be null
	Name     string // protocol name of the argument
	// Interface is the interface of an object or new_id argument. It is empty
	// for an untyped new_id (wl_registry.bind) and for an object argument
	// without a declared interface.
	Interface string
}

// Message describes one request or event of an interface. The position of a
// Message in [Interface.Requests] or [Interface.Events] is its opcode.
type Message struct {
	Name       string
	Since      uint32 // first interface version that has the message
	Destructor bool   // a request that destroys the object
	Args       []Arg
}

// Interface is the generated description of a Wayland interface. The runtime
// uses it to find opcodes, check versions and skip events of destroyed
// objects without a typed decoder.
type Interface struct {
	Name     string
	Version  uint32 // highest version known to the generated bindings
	Requests []Message
	Events   []Message
}

var registry = map[string]*Interface{}

// Register makes an interface known by name. Generated code calls it from
// init so the runtime can resolve the interface of a new_id argument when it
// has to skip an event of a destroyed object. It panics on duplicate names.
func Register(i *Interface) {
	if _, dup := registry[i.Name]; dup {
		panic("wayland: interface " + i.Name + " registered twice")
	}
	registry[i.Name] = i
}

// Lookup returns the registered interface called name, or nil.
func Lookup(name string) *Interface { return registry[name] }
