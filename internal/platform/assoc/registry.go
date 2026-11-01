package assoc

import (
	"slices"
	"strings"
	"sync"
)

// Registry is the part of the Windows registry the Windows implementation
// uses: string values of keys below HKEY_CURRENT_USER, named by paths with
// '\' ("Software\Classes\notes\shell\open\command"). Key and value names
// are case-insensitive, as in the registry. The real one (Windows only)
// uses advapi32's RegCreateKeyExW, RegQueryValueExW, RegSetValueExW,
// RegEnumKeyExW, RegEnumValueW, RegDeleteValueW and RegDeleteTreeW; tests
// use a MemRegistry.
type Registry interface {
	// String returns the string value name of key ("" names the key's
	// default value). ok is false when the key or the value does not
	// exist.
	String(key, name string) (value string, ok bool, err error)
	// SetString creates key and its parents when needed and sets the
	// value name to value (REG_SZ).
	SetString(key, name, value string) error
	// SubKeys returns the names of the direct subkeys of key, nil when it
	// does not exist.
	SubKeys(key string) ([]string, error)
	// ValueNames returns the names of the values of key ("" for the
	// default value), nil when it does not exist.
	ValueNames(key string) ([]string, error)
	// DeleteValue removes a value; a missing one is not an error.
	DeleteValue(key, name string) error
	// DeleteKey removes key and everything below it; a missing one is not
	// an error.
	DeleteKey(key string) error
}

// MemRegistry is an in-memory Registry for tests. Writes counts the calls
// that changed it, so tests check that nothing is written when nothing
// changes. The zero value is empty and ready; it is safe for concurrent
// use.
type MemRegistry struct {
	mu     sync.Mutex
	keys   map[string]*memKey // by lower-cased path
	writes int
	shell  memShell // the Shell and MachineReader side (shell.go)
}

type memKey struct {
	path   string            // as first created
	values map[string]string // by lower-cased name
	names  map[string]string // lower-cased name -> name as first set
}

var _ Registry = (*MemRegistry)(nil)

func lowerKey(key string) string { return strings.ToLower(strings.Trim(key, `\`)) }

func (r *MemRegistry) String(key, name string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.keys[lowerKey(key)]
	if k == nil {
		return "", false, nil
	}
	v, ok := k.values[strings.ToLower(name)]
	return v, ok, nil
}

func (r *MemRegistry) SetString(key, name, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.keys == nil {
		r.keys = map[string]*memKey{}
	}
	path := strings.Trim(key, `\`)
	parts := strings.Split(path, `\`)
	created := false
	for i := range parts {
		p := strings.Join(parts[:i+1], `\`)
		if r.keys[strings.ToLower(p)] == nil {
			r.keys[strings.ToLower(p)] = &memKey{path: p, values: map[string]string{}, names: map[string]string{}}
			created = true
		}
	}
	k := r.keys[strings.ToLower(path)]
	ln := strings.ToLower(name)
	if old, ok := k.values[ln]; ok && old == value && !created {
		return nil
	}
	if _, ok := k.names[ln]; !ok {
		k.names[ln] = name
	}
	k.values[ln] = value
	r.writes++
	return nil
}

func (r *MemRegistry) SubKeys(key string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lk := lowerKey(key)
	if r.keys[lk] == nil {
		return nil, nil
	}
	var out []string
	for p, k := range r.keys {
		if rest, ok := strings.CutPrefix(p, lk+`\`); ok && !strings.Contains(rest, `\`) {
			out = append(out, k.path[strings.LastIndex(k.path, `\`)+1:])
		}
	}
	slices.Sort(out)
	return out, nil
}

func (r *MemRegistry) ValueNames(key string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.keys[lowerKey(key)]
	if k == nil {
		return nil, nil
	}
	var out []string
	for _, n := range k.names {
		out = append(out, n)
	}
	slices.Sort(out)
	return out, nil
}

func (r *MemRegistry) DeleteValue(key, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.keys[lowerKey(key)]
	ln := strings.ToLower(name)
	if k == nil {
		return nil
	}
	if _, ok := k.values[ln]; !ok {
		return nil
	}
	delete(k.values, ln)
	delete(k.names, ln)
	r.writes++
	return nil
}

func (r *MemRegistry) DeleteKey(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	lk := lowerKey(key)
	if r.keys[lk] == nil {
		return nil
	}
	for p := range r.keys {
		if p == lk || strings.HasPrefix(p, lk+`\`) {
			delete(r.keys, p)
		}
	}
	r.writes++
	return nil
}

// Writes returns how many calls changed the registry.
func (r *MemRegistry) Writes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writes
}

// Dump returns every value as "key\name=value" lines, sorted, for test
// failure messages and golden comparisons (names as first written).
func (r *MemRegistry) Dump() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var lines []string
	for _, k := range r.keys {
		for ln, v := range k.values {
			lines = append(lines, k.path+`\`+k.names[ln]+"="+v)
		}
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}
