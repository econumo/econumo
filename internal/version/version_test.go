package version

import "testing"

func TestDefaultIsDev(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q, want %q (the unstamped default)", Version, "dev")
	}
}

func TestRefPrefersTheStampedCommit(t *testing.T) {
	old := Commit
	t.Cleanup(func() { Commit = old })
	Commit = "abc123"
	if got := Ref(); got != "abc123" {
		t.Fatalf("Ref() = %q, want %q", got, "abc123")
	}
}
