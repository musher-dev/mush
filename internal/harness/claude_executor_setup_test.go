//go:build unix

package harness

import (
	"context"
	"testing"
	"time"
)

func TestClaudeSetupBundleLoadDoesNotBlockOnReady(t *testing.T) {
	exec := NewClaudeExecutor()
	waitStarted := make(chan struct{})
	releaseReady := make(chan struct{})
	onReadyCalled := make(chan struct{}, 1)

	exec.startPTYFunc = func(context.Context) error { return nil }
	exec.startOutputFunc = func() {}
	exec.waitForReadyFunc = func(context.Context) {
		close(waitStarted)
		<-releaseReady
	}

	done := make(chan error, 1)

	go func() {
		done <- exec.Setup(t.Context(), &SetupOptions{
			BundleLoadMode: true,
			OnReady: func() {
				onReadyCalled <- struct{}{}
			},
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Setup() error = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Setup() blocked in bundle load mode, want non-blocking")
	}

	select {
	case <-waitStarted:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("waitForReady was not started asynchronously")
	}

	select {
	case <-onReadyCalled:
		t.Fatal("OnReady called before readiness gate released")
	default:
	}

	close(releaseReady)

	select {
	case <-onReadyCalled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("OnReady not called after readiness gate released")
	}
}

func TestClaudeSetupLinkModeBlocksUntilReady(t *testing.T) {
	exec := NewClaudeExecutor()
	waitStarted := make(chan struct{})
	releaseReady := make(chan struct{})
	onReadyCalled := make(chan struct{}, 1)

	exec.startPTYFunc = func(context.Context) error { return nil }
	exec.startOutputFunc = func() {}
	exec.waitForReadyFunc = func(context.Context) {
		close(waitStarted)
		<-releaseReady
	}

	done := make(chan error, 1)

	go func() {
		done <- exec.Setup(t.Context(), &SetupOptions{
			BundleLoadMode: false,
			OnReady: func() {
				onReadyCalled <- struct{}{}
			},
		})
	}()

	select {
	case <-waitStarted:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("waitForReady did not run in link mode")
	}

	select {
	case err := <-done:
		t.Fatalf("Setup() returned early with %v, expected block until ready", err)
	default:
	}

	close(releaseReady)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Setup() error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Setup() did not return after readiness gate released")
	}

	select {
	case <-onReadyCalled:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("OnReady not called in link mode after readiness")
	}
}
