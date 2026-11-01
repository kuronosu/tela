package assoc

// Shell is implemented by a Registry that can also act on the Windows
// shell, which the Windows associations need after a change and for
// MakeDefault. A Registry that is not a Shell (a bare registry) makes
// both do nothing; the real registry and the MemRegistry implement it.
type Shell interface {
	// AssociationsChanged tells the shell that file type or URL
	// associations changed, so Explorer refreshes its icons and handlers
	// (SHChangeNotify with SHCNE_ASSOCCHANGED).
	AssociationsChanged()
	// OpenSettings opens a Settings page by its URI
	// ("ms-settings:defaultapps"), as ShellExecuteW "open" does.
	OpenSettings(uri string) error
}

// MachineReader is implemented by a Registry that can read, never write,
// HKEY_LOCAL_MACHINE. The Windows associations use it to find what the
// system or an installer for all users registered: a per-user key shadows
// the machine's, so writing one over another application's would take it
// over. The key names are relative to the hive, like Registry's.
type MachineReader interface {
	// MachineString is Registry.String for HKEY_LOCAL_MACHINE.
	MachineString(key, name string) (value string, ok bool, err error)
}

// memShell is the state of a MemRegistry as a Shell and MachineReader.
type memShell struct {
	machine  map[string]map[string]string // lower-cased key -> lower-cased value name -> value
	notified int
	opened   []string
	openErr  error
}

var (
	_ Shell         = (*MemRegistry)(nil)
	_ MachineReader = (*MemRegistry)(nil)
)

// SetMachineString plays an installer for all users (or the system):
// it sets a value below HKEY_LOCAL_MACHINE for MachineString to find.
func (r *MemRegistry) SetMachineString(key, name, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shell.machine == nil {
		r.shell.machine = map[string]map[string]string{}
	}
	k := lowerKey(key)
	if r.shell.machine[k] == nil {
		r.shell.machine[k] = map[string]string{}
	}
	r.shell.machine[k][lowerKey(name)] = value
}

// MachineString returns a value set with SetMachineString.
func (r *MemRegistry) MachineString(key, name string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.shell.machine[lowerKey(key)][lowerKey(name)]
	return v, ok, nil
}

// AssociationsChanged counts the notification (Notified).
func (r *MemRegistry) AssociationsChanged() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shell.notified++
}

// OpenSettings records the URI (Opened), or returns the error set with
// SetOpenError.
func (r *MemRegistry) OpenSettings(uri string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shell.openErr != nil {
		return r.shell.openErr
	}
	r.shell.opened = append(r.shell.opened, uri)
	return nil
}

// SetOpenError makes OpenSettings fail with err until SetOpenError(nil).
func (r *MemRegistry) SetOpenError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shell.openErr = err
}

// Notified returns how many times AssociationsChanged was called.
func (r *MemRegistry) Notified() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shell.notified
}

// Opened returns the Settings URIs OpenSettings opened, in order.
func (r *MemRegistry) Opened() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.shell.opened...)
}
