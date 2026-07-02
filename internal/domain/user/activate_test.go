package user

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// TestUserActivateDeactivate covers the activate/deactivate mutators: state flips
// and updatedAt is bumped only on a real change (idempotent no-ops leave it).
func TestUserActivateDeactivate(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)
	t3 := t0.Add(3 * time.Hour)

	u := FromState(vo.NewId(), "ident", "enc-email", "Name", "avatar", "pwhash", "salt",
		true, t0, t0, nil)

	// Already active: Activate is a no-op (updatedAt unchanged).
	u.Activate(t1)
	if !u.IsActive() {
		t.Fatal("user should still be active")
	}
	if !u.UpdatedAt().Equal(t0) {
		t.Errorf("no-op Activate bumped updatedAt: got %v, want %v", u.UpdatedAt(), t0)
	}

	// Deactivate: state flips, updatedAt bumps.
	u.Deactivate(t1)
	if u.IsActive() {
		t.Error("user should be inactive after Deactivate")
	}
	if !u.UpdatedAt().Equal(t1) {
		t.Errorf("Deactivate updatedAt: got %v, want %v", u.UpdatedAt(), t1)
	}

	// Already inactive: Deactivate is a no-op (updatedAt unchanged).
	u.Deactivate(t2)
	if !u.UpdatedAt().Equal(t1) {
		t.Errorf("no-op Deactivate bumped updatedAt: got %v, want %v", u.UpdatedAt(), t1)
	}

	// Reactivate: state flips back, updatedAt bumps.
	u.Activate(t3)
	if !u.IsActive() {
		t.Error("user should be active after Activate")
	}
	if !u.UpdatedAt().Equal(t3) {
		t.Errorf("Activate updatedAt: got %v, want %v", u.UpdatedAt(), t3)
	}
}
