//go:build unix

package harness

import (
	"testing"
)

func TestRegisteredNamesIncludesBuiltins(t *testing.T) {
	names := RegisteredNames()

	has := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}

		return false
	}

	// On unix builds, built-ins should be registered via init().
	if !has("claude") {
		t.Error("expected 'claude' in RegisteredNames()")
	}

	if !has("codex") {
		t.Error("expected 'codex' in RegisteredNames()")
	}

	if !has("copilot") {
		t.Error("expected 'copilot' in RegisteredNames()")
	}

	if !has("cursor") {
		t.Error("expected 'cursor' in RegisteredNames()")
	}

	if !has("gemini") {
		t.Error("expected 'gemini' in RegisteredNames()")
	}

	if !has("opencode") {
		t.Error("expected 'opencode' in RegisteredNames()")
	}
}

func TestLookupFindsRegistered(t *testing.T) {
	info, ok := Lookup("claude")
	if !ok {
		t.Fatal("Lookup('claude') = false, want true")
	}

	if info.Name != "claude" {
		t.Fatalf("Lookup('claude').Name = %q, want 'claude'", info.Name)
	}
}

func TestLookupReturnsFalseForUnknown(t *testing.T) {
	_, ok := Lookup("nonexistent")
	if ok {
		t.Fatal("Lookup('nonexistent') = true, want false")
	}
}

// TestRegistryAndCatalogParity ensures every registered executor also has a
// provider spec, and vice-versa. This catches drift between the two systems.
func TestRegistryAndCatalogParity(t *testing.T) {
	registeredNames := RegisteredNames()
	providerNames := ProviderNames()

	if len(registeredNames) != len(providerNames) {
		t.Fatalf("RegisteredNames()=%v ProviderNames()=%v — counts differ", registeredNames, providerNames)
	}

	for i := range registeredNames {
		if registeredNames[i] != providerNames[i] {
			t.Fatalf("name mismatch at [%d]: registered=%q provider=%q", i, registeredNames[i], providerNames[i])
		}
	}
}
