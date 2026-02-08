package claude

import "testing"

func TestAvailable(t *testing.T) {
	// This test just verifies the function doesn't panic
	// The result depends on whether 'claude' is in PATH
	_ = Available()
}
