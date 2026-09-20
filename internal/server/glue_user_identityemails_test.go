package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

// A nil oauth service is what a container that wires no oauth slot hands in;
// the port must degrade to "no copies" rather than panic on the mail path.
func TestIdentityEmailLister_NilServiceYieldsNoAddresses(t *testing.T) {
	got, err := NewIdentityEmailLister(nil).ListEmails(context.Background(), vo.NewId())
	if err != nil {
		t.Fatalf("ListEmails: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("emails = %v, want none", got)
	}
}
