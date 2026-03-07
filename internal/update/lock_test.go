package update

import "testing"

func TestWithAgentLock_Serializes(t *testing.T) {
	tmp := t.TempDir()
	setTestHome(t, tmp)

	if err := WithAgentLock(func() error {
		called := false
		if err := WithAgentLock(func() error {
			called = true
			return nil
		}); err != nil {
			return err
		}

		if called {
			t.Fatal("inner lock should not run while outer lock is held")
		}

		return nil
	}); err != nil {
		t.Fatalf("WithAgentLock returned error: %v", err)
	}

	ran := false
	if err := WithAgentLock(func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("second WithAgentLock returned error: %v", err)
	}

	if !ran {
		t.Fatal("expected callback to run after lock release")
	}
}
