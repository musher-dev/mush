package prompt

import (
	"errors"
	"testing"
)

func TestIsCanceled(t *testing.T) {
	if !IsCanceled(errCanceled) {
		t.Fatal("IsCanceled(errCanceled) = false, want true")
	}

	if !IsCanceled(errors.Join(errors.New("other"), errCanceled)) {
		t.Fatal("IsCanceled(wrapped errCanceled) = false, want true")
	}

	if IsCanceled(errors.New("not canceled")) {
		t.Fatal("IsCanceled(unrelated error) = true, want false")
	}
}
