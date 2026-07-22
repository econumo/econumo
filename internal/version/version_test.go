package version

import "testing"

func TestDefaultIsDev(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q, want %q (the unstamped default)", Version, "dev")
	}
}
