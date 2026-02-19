package harness

import (
	"fmt"
	"sort"
	"sync"
)

// Info describes a registered harness type.
type Info struct {
	// Name is the harness type identifier (e.g., "claude", "bash", "codex").
	Name string

	// Available reports whether the harness runtime is installed.
	Available func() bool

	// New creates a new Executor instance for this harness type.
	New func() Executor
}

var (
	registryMu sync.Mutex
	registry   = map[string]Info{}
)

// Register adds a harness type to the global registry. Panics on duplicate names.
func Register(info Info) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, dup := registry[info.Name]; dup {
		panic(fmt.Sprintf("harness: duplicate registration for %q", info.Name))
	}

	registry[info.Name] = info
}

// Lookup returns the Info for a registered harness type.
func Lookup(name string) (Info, bool) {
	registryMu.Lock()
	defer registryMu.Unlock()

	info, ok := registry[name]

	return info, ok
}

// RegisteredNames returns all registered harness type names in sorted order.
func RegisteredNames() []string {
	registryMu.Lock()
	defer registryMu.Unlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// AvailableNames returns harness type names where Available() == true, sorted.
func AvailableNames() []string {
	registryMu.Lock()
	defer registryMu.Unlock()

	names := make([]string, 0, len(registry))
	for name, info := range registry {
		if info.Available() {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return names
}
