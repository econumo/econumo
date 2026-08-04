package model

import (
	"testing"
	"time"
)

var (
	tp0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	tp1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	tp2 = time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
)

func newPayee(t *testing.T) *Payee {
	return NewPayee(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "Amazon", tp0)
}

func TestNewPayee_Defaults(t *testing.T) {
	p := newPayee(t)
	if p.Name != "Amazon" {
		t.Errorf("name=%q want Amazon", p.Name)
	}
	if p.SortKey != "" {
		t.Errorf("a new entity must carry no sort key, got %q", p.SortKey)
	}
	if p.IsArchived {
		t.Error("new payee must not be archived")
	}
	if !p.CreatedAt.Equal(tp0) || !p.UpdatedAt.Equal(tp0) {
		t.Errorf("timestamps: created=%v updated=%v want both %v", p.CreatedAt, p.UpdatedAt, tp0)
	}
}

func TestNewPayee_Getters(t *testing.T) {
	id := mustID(t, "11111111-1111-1111-1111-111111111111")
	uid := mustID(t, "22222222-2222-2222-2222-222222222222")
	p := NewPayee(id, uid, "Amazon", tp0)
	if !p.ID.Equal(id) {
		t.Errorf("ID=%v want %v", p.ID, id)
	}
	if !p.UserID.Equal(uid) {
		t.Errorf("UserID=%v want %v", p.UserID, uid)
	}
}

func TestPayee_FromState_RoundTrip(t *testing.T) {
	id := mustID(t, "11111111-1111-1111-1111-111111111111")
	uid := mustID(t, "22222222-2222-2222-2222-222222222222")
	p := &Payee{ID: id, UserID: uid, Name: "Costco", SortKey: "c007", IsArchived: true, CreatedAt: tp0, UpdatedAt: tp1}
	if !p.ID.Equal(id) || !p.UserID.Equal(uid) {
		t.Fatal("ids did not round-trip")
	}
	if p.Name != "Costco" {
		t.Errorf("name=%q want Costco", p.Name)
	}
	if p.SortKey != "c007" {
		t.Errorf("position=%q want 7", p.SortKey)
	}
	if !p.IsArchived {
		t.Error("isArchived should round-trip as true")
	}
	if !p.CreatedAt.Equal(tp0) || !p.UpdatedAt.Equal(tp1) {
		t.Errorf("timestamps did not round-trip: %v / %v", p.CreatedAt, p.UpdatedAt)
	}
}

func TestPayee_SetSortKey_NoBump(t *testing.T) {
	p := newPayee(t)
	p.SetSortKey("c005")
	if p.SortKey != "c005" {
		t.Errorf("position=%q want 5", p.SortKey)
	}
	// SetSortKey is construction-time and must NOT bump updatedAt.
	if !p.UpdatedAt.Equal(tp0) {
		t.Errorf("SetSortKey bumped updatedAt to %v", p.UpdatedAt)
	}
}

func TestPayee_UpdateName_OnlyBumpsOnChange(t *testing.T) {
	p := newPayee(t)
	p.UpdateName("Amazon", tp1) // no-op
	if !p.UpdatedAt.Equal(tp0) {
		t.Fatal("same-name update bumped updatedAt")
	}
	p.UpdateName("eBay", tp1)
	if p.Name != "eBay" || !p.UpdatedAt.Equal(tp1) {
		t.Fatalf("rename: %q / %v", p.Name, p.UpdatedAt)
	}
}

func TestPayee_UpdateSortKey_OnlyBumpsOnChange(t *testing.T) {
	p := newPayee(t)
	p.UpdateSortKey("", tp1) // same as default (unset) -> no-op
	if !p.UpdatedAt.Equal(tp0) {
		t.Fatal("same-key update bumped updatedAt")
	}
	p.UpdateSortKey("c003", tp1)
	if p.SortKey != "c003" || !p.UpdatedAt.Equal(tp1) {
		t.Fatalf("key change: %q / %v", p.SortKey, p.UpdatedAt)
	}
}

func TestPayee_Archive_Unarchive_OnlyBumpOnChange(t *testing.T) {
	p := newPayee(t)
	p.Unarchive(tp1) // already unarchived -> no-op
	if !p.UpdatedAt.Equal(tp0) {
		t.Fatal("no-op unarchive bumped updatedAt")
	}
	p.Archive(tp1)
	if !p.IsArchived || !p.UpdatedAt.Equal(tp1) {
		t.Fatalf("archive: archived=%v updatedAt=%v", p.IsArchived, p.UpdatedAt)
	}
	p.Archive(tp2) // re-archive -> no-op
	if !p.UpdatedAt.Equal(tp1) {
		t.Fatalf("re-archive bumped updatedAt to %v", p.UpdatedAt)
	}
	p.Unarchive(tp2)
	if p.IsArchived || !p.UpdatedAt.Equal(tp2) {
		t.Fatalf("unarchive: archived=%v updatedAt=%v", p.IsArchived, p.UpdatedAt)
	}
}
