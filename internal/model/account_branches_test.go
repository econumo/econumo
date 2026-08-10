package model

import (
	"testing"
	"time"
)

func TestNewAccount_Getters(t *testing.T) {
	id := mustID(t, "11111111-1111-1111-1111-111111111111")
	uid := mustID(t, "22222222-2222-2222-2222-222222222222")
	cid := mustID(t, "33333333-3333-3333-3333-333333333333")
	a := NewAccount(id, uid, cid, "Wallet", "wallet", ta0)
	if !a.ID.Equal(id) || !a.UserID.Equal(uid) || !a.CurrencyID.Equal(cid) {
		t.Error("ids did not round-trip through NewAccount")
	}
	if a.Name != "Wallet" || a.Icon != "wallet" {
		t.Errorf("name/icon: %q / %q", a.Name, a.Icon)
	}
	if !a.CreatedAt.Equal(ta0) || !a.UpdatedAt.Equal(ta0) {
		t.Errorf("timestamps: %v / %v", a.CreatedAt, a.UpdatedAt)
	}
}

func TestAccount_FromState_RoundTrip(t *testing.T) {
	id := mustID(t, "11111111-1111-1111-1111-111111111111")
	uid := mustID(t, "22222222-2222-2222-2222-222222222222")
	cid := mustID(t, "33333333-3333-3333-3333-333333333333")
	a := &Account{ID: id, UserID: uid, CurrencyID: cid, Name: "Savings", Type: TypeCash, Icon: "piggy", IsDeleted: true, CreatedAt: ta0, UpdatedAt: ta1}
	if a.Type != TypeCash {
		t.Errorf("type=%d want cash", a.Type)
	}
	if !a.IsDeleted {
		t.Error("isDeleted should round-trip as true")
	}
	if a.Name != "Savings" || a.Icon != "piggy" {
		t.Errorf("name/icon: %q / %q", a.Name, a.Icon)
	}
	if !a.CreatedAt.Equal(ta0) || !a.UpdatedAt.Equal(ta1) {
		t.Errorf("timestamps: %v / %v", a.CreatedAt, a.UpdatedAt)
	}
}

func TestAccount_UpdateName_OnlyBumpsOnChange(t *testing.T) {
	a := newAcct(t)
	a.UpdateName("Cash", ta1) // same -> no-op
	if !a.UpdatedAt.Equal(ta0) {
		t.Fatal("same-name update bumped updatedAt")
	}
	a.UpdateName("Bank", ta1)
	if a.Name != "Bank" || !a.UpdatedAt.Equal(ta1) {
		t.Fatalf("rename: %q / %v", a.Name, a.UpdatedAt)
	}
}

func newFolder(t *testing.T) *Folder {
	return NewFolder(
		mustID(t, "44444444-4444-4444-4444-444444444444"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "Daily", ta0)
}

func TestNewFolder_Defaults(t *testing.T) {
	f := newFolder(t)
	if f.Name != "Daily" {
		t.Errorf("name=%q want Daily", f.Name)
	}
	if f.SortKey != "" {
		t.Errorf("a new entity must carry no sort key, got %q", f.SortKey)
	}
	if !f.IsVisible {
		t.Error("new folder must be visible")
	}
	if !f.CreatedAt.Equal(ta0) || !f.UpdatedAt.Equal(ta0) {
		t.Errorf("timestamps: %v / %v", f.CreatedAt, f.UpdatedAt)
	}
}

func TestFolder_Getters(t *testing.T) {
	id := mustID(t, "44444444-4444-4444-4444-444444444444")
	uid := mustID(t, "22222222-2222-2222-2222-222222222222")
	f := NewFolder(id, uid, "Daily", ta0)
	if !f.ID.Equal(id) || !f.UserID.Equal(uid) {
		t.Error("folder ids did not round-trip")
	}
}

func TestFolder_FromState_RoundTrip(t *testing.T) {
	id := mustID(t, "44444444-4444-4444-4444-444444444444")
	uid := mustID(t, "22222222-2222-2222-2222-222222222222")
	f := &Folder{ID: id, UserID: uid, Name: "Hidden", SortKey: "c009", IsVisible: false, CreatedAt: ta0, UpdatedAt: ta1}
	if f.SortKey != "c009" {
		t.Errorf("position=%q want 9", f.SortKey)
	}
	if f.IsVisible {
		t.Error("isVisible should round-trip as false")
	}
	if f.Name != "Hidden" {
		t.Errorf("name=%q", f.Name)
	}
	if !f.CreatedAt.Equal(ta0) || !f.UpdatedAt.Equal(ta1) {
		t.Errorf("timestamps: %v / %v", f.CreatedAt, f.UpdatedAt)
	}
}

func TestFolder_SetSortKey_NoBump(t *testing.T) {
	f := newFolder(t)
	f.SetSortKey("c004")
	if f.SortKey != "c004" {
		t.Errorf("position=%q want 4", f.SortKey)
	}
	if !f.UpdatedAt.Equal(ta0) {
		t.Errorf("SetSortKey bumped updatedAt to %v", f.UpdatedAt)
	}
}

func TestFolder_UpdateName_OnlyBumpsOnChange(t *testing.T) {
	f := newFolder(t)
	f.UpdateName("Daily", ta1) // no-op
	if !f.UpdatedAt.Equal(ta0) {
		t.Fatal("same-name update bumped updatedAt")
	}
	f.UpdateName("Weekly", ta1)
	if f.Name != "Weekly" || !f.UpdatedAt.Equal(ta1) {
		t.Fatalf("rename: %q / %v", f.Name, f.UpdatedAt)
	}
}

func TestFolder_UpdateSortKey_OnlyBumpsOnChange(t *testing.T) {
	f := newFolder(t)
	f.UpdateSortKey("", ta1) // same -> no-op
	if !f.UpdatedAt.Equal(ta0) {
		t.Fatal("same-key update bumped updatedAt")
	}
	f.UpdateSortKey("c002", ta1)
	if f.SortKey != "c002" || !f.UpdatedAt.Equal(ta1) {
		t.Fatalf("key change: %q / %v", f.SortKey, f.UpdatedAt)
	}
}

func TestFolder_Visibility_OnlyBumpsOnChange(t *testing.T) {
	f := newFolder(t)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	f.MakeVisible(ta1) // already visible -> no-op
	if !f.UpdatedAt.Equal(ta0) {
		t.Fatal("no-op MakeVisible bumped updatedAt")
	}
	f.MakeInvisible(ta1)
	if f.IsVisible || !f.UpdatedAt.Equal(ta1) {
		t.Fatalf("MakeInvisible: visible=%v updatedAt=%v", f.IsVisible, f.UpdatedAt)
	}
	f.MakeInvisible(t2) // already invisible -> no-op
	if !f.UpdatedAt.Equal(ta1) {
		t.Fatalf("re-MakeInvisible bumped updatedAt to %v", f.UpdatedAt)
	}
	f.MakeVisible(t2)
	if !f.IsVisible || !f.UpdatedAt.Equal(t2) {
		t.Fatalf("MakeVisible: visible=%v updatedAt=%v", f.IsVisible, f.UpdatedAt)
	}
}
