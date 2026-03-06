package harness

import (
	"sort"
	"sync"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// providerSpecs stores provider specs registered by builtins.
var (
	providerSpecsMu sync.RWMutex
	providerSpecs   = map[string]*harnesstype.ProviderSpec{}
)

// registerProviderSpec adds a provider spec to the provider map.
func registerProviderSpec(spec *harnesstype.ProviderSpec) {
	providerSpecsMu.Lock()
	defer providerSpecsMu.Unlock()

	providerSpecs[spec.Name] = spec
}

// GetProvider returns the ProviderSpec for a named harness type.
func GetProvider(name string) (*harnesstype.ProviderSpec, bool) {
	providerSpecsMu.RLock()
	defer providerSpecsMu.RUnlock()

	spec, ok := providerSpecs[name]

	return spec, ok
}

// ProviderNames returns all provider names in sorted order.
func ProviderNames() []string {
	providerSpecsMu.RLock()
	defer providerSpecsMu.RUnlock()

	names := make([]string, 0, len(providerSpecs))
	for name := range providerSpecs {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// HasAssetMapping returns true if the named provider has asset mapping rules.
func HasAssetMapping(name string) bool {
	providerSpecsMu.RLock()
	defer providerSpecsMu.RUnlock()

	spec, ok := providerSpecs[name]

	return ok && spec.Assets != nil
}

// AvailableFunc returns a lazy closure that checks if a provider's binary is available.
// The closure reads from the provider spec map at call time, avoiding init-order dependence.
func AvailableFunc(name string) func() bool {
	return func() bool {
		providerSpecsMu.RLock()

		spec, ok := providerSpecs[name]

		providerSpecsMu.RUnlock()

		if !ok {
			return false
		}

		return harnesstype.AvailableFunc(spec)()
	}
}
