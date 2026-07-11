package errs_test

import (
	"fmt"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

func TestTooManyRequests(t *testing.T) {
	e := errs.NewTooManyRequests("Too many attempts. Try again later.")
	if e.Error() != "Too many attempts. Try again later." {
		t.Fatalf("Error() = %q", e.Error())
	}
	if _, ok := errs.AsTooManyRequests(e); !ok {
		t.Fatal("AsTooManyRequests(e) = false, want true")
	}
	if _, ok := errs.AsTooManyRequests(fmt.Errorf("wrap: %w", e)); !ok {
		t.Fatal("AsTooManyRequests(wrapped) = false, want true")
	}
	if _, ok := errs.AsTooManyRequests(fmt.Errorf("other")); ok {
		t.Fatal("AsTooManyRequests(other) = true, want false")
	}
	empty := &errs.TooManyRequestsError{}
	if empty.Error() != "too many requests" {
		t.Fatalf("zero-value Error() = %q, want %q", empty.Error(), "too many requests")
	}
}
