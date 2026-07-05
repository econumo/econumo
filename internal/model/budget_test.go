package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	t0 = time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)
	t1 = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
)

func TestElementType_AliasRoundTrip(t *testing.T) {
	cases := []struct {
		typ   ElementType
		alias string
		num   int16
	}{
		{ElementEnvelope, "envelope", 0},
		{ElementCategory, "category", 1},
		{ElementTag, "tag", 2},
	}
	for _, c := range cases {
		if c.typ.Alias() != c.alias {
			t.Errorf("%d.Alias()=%q want %q", c.typ, c.typ.Alias(), c.alias)
		}
		if c.typ.Int16() != c.num {
			t.Errorf("%s.Int16()=%d want %d", c.alias, c.typ.Int16(), c.num)
		}
		got, err := ElementTypeFromAlias(c.alias)
		if err != nil {
			t.Errorf("ElementTypeFromAlias(%q) err: %v", c.alias, err)
		}
		if got != c.typ {
			t.Errorf("ElementTypeFromAlias(%q)=%d want %d", c.alias, got, c.typ)
		}
	}
}

func TestElementTypeFromAlias_Invalid(t *testing.T) {
	_, err := ElementTypeFromAlias("nope")
	if err == nil {
		t.Fatal("want error for unknown alias")
	}
	var ve *errs.ValidationError
	if !asValidation(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
}

func TestElementType_AliasOutOfRange(t *testing.T) {
	if ElementType(99).Alias() != "" {
		t.Errorf("out-of-range alias should be empty, got %q", ElementType(99).Alias())
	}
	if ElementType(-1).Alias() != "" {
		t.Errorf("negative alias should be empty, got %q", ElementType(-1).Alias())
	}
}

func TestBudgetRole_Values(t *testing.T) {
	if BudgetRoleOwner != -1 || BudgetRoleAdmin != 0 || BudgetRoleUser != 1 || BudgetRoleGuest != 2 {
		t.Fatalf("role values drifted: owner=%d admin=%d user=%d guest=%d", BudgetRoleOwner, BudgetRoleAdmin, BudgetRoleUser, BudgetRoleGuest)
	}
}

func TestBudgetRole_AliasRoundTrip_StoredRoles(t *testing.T) {
	for _, c := range []struct {
		role  BudgetRole
		alias string
	}{
		{BudgetRoleAdmin, "admin"}, {BudgetRoleUser, "user"}, {BudgetRoleGuest, "guest"},
	} {
		if c.role.Alias() != c.alias {
			t.Errorf("%d.Alias()=%q want %q", c.role, c.role.Alias(), c.alias)
		}
		got, err := BudgetRoleFromAlias(c.alias)
		if err != nil {
			t.Errorf("BudgetRoleFromAlias(%q) err: %v", c.alias, err)
		}
		if got != c.role {
			t.Errorf("BudgetRoleFromAlias(%q)=%d want %d", c.alias, got, c.role)
		}
	}
}

func TestBudgetRole_OwnerAliasIsPresentationOnly(t *testing.T) {
	// owner has a wire alias...
	if BudgetRoleOwner.Alias() != "owner" {
		t.Errorf("BudgetRoleOwner.Alias()=%q want owner", BudgetRoleOwner.Alias())
	}
	// ...but is NOT a valid INPUT role (BudgetRoleFromAlias rejects it).
	if _, err := BudgetRoleFromAlias("owner"); err == nil {
		t.Fatal("BudgetRoleFromAlias(owner) must error (presentation-only)")
	}
}

func TestBudgetRoleFromAlias_Invalid(t *testing.T) {
	if _, err := BudgetRoleFromAlias("superuser"); err == nil {
		t.Fatal("want error for unknown role alias")
	}
}

func TestValidateName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"ab", false}, // 2 < 3
		{"abc", true}, // 3 = min
		{"a budget name", true},
		{string(make([]rune, 64)), true},  // exactly 64
		{string(make([]rune, 65)), false}, // 65 > 64
		{"", false},
	}
	for _, c := range cases {
		err := ValidateName("Budget", c.name)
		if c.ok && err != nil {
			t.Errorf("ValidateName(len=%d) unexpected err: %v", len([]rune(c.name)), err)
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateName(len=%d) expected err", len([]rune(c.name)))
		}
	}
}

func TestValidateName_RuneCountedNotBytes(t *testing.T) {
	// 3 multibyte runes (each 2+ bytes) must pass the 3-char minimum.
	if err := ValidateName("Folder", "ααα"); err != nil {
		t.Fatalf("3 multibyte runes should be valid: %v", err)
	}
	// 2 multibyte runes must fail (rune count, not byte count).
	if err := ValidateName("Folder", "αα"); err == nil {
		t.Fatal("2 runes must fail even though >2 bytes")
	}
}

func TestValidateName_MessageUsesLabel(t *testing.T) {
	err := ValidateName("Envelope", "x")
	var ve *errs.ValidationError
	if !asValidation(err, &ve) {
		t.Fatalf("want validation error, got %T", err)
	}
	if len(ve.Fields) == 0 || ve.Fields[0].Message != "Envelope name must be 3-64 characters" {
		t.Fatalf("message=%+v want 'Envelope name must be 3-64 characters'", ve.Fields)
	}
}

func TestNewBudget_SnapsStartToFirstOfMonth(t *testing.T) {
	b := NewBudget(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		"My Budget", mustID(t, "33333333-3333-3333-3333-333333333333"), t0, t0)
	want := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	if !b.StartedAt.Equal(want) {
		t.Fatalf("startedAt=%v want %v (first of month)", b.StartedAt, want)
	}
}

func TestBudget_UpdateName_OnlyBumpsOnChange(t *testing.T) {
	b := NewBudget(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "Original",
		mustID(t, "33333333-3333-3333-3333-333333333333"), t0, t0)
	// no-op update keeps updatedAt.
	b.UpdateName("Original", t1)
	if !b.UpdatedAt.Equal(t0) {
		t.Fatalf("no-op rename bumped updatedAt to %v", b.UpdatedAt)
	}
	// real change bumps it.
	b.UpdateName("Renamed", t1)
	if !b.UpdatedAt.Equal(t1) || b.Name != "Renamed" {
		t.Fatalf("rename: name=%q updatedAt=%v", b.Name, b.UpdatedAt)
	}
}

func TestBudget_UpdateCurrency_OnlyBumpsOnChange(t *testing.T) {
	cur := mustID(t, "33333333-3333-3333-3333-333333333333")
	other := mustID(t, "44444444-4444-4444-4444-444444444444")
	b := NewBudget(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "B", cur, t0, t0)
	b.UpdateCurrency(cur, t1)
	if !b.UpdatedAt.Equal(t0) {
		t.Fatal("same-currency update bumped updatedAt")
	}
	b.UpdateCurrency(other, t1)
	if !b.CurrencyID.Equal(other) || !b.UpdatedAt.Equal(t1) {
		t.Fatalf("currency change not applied: %v / %v", b.CurrencyID, b.UpdatedAt)
	}
}

func TestBudget_StartFrom_SnapsAndAlwaysBumps(t *testing.T) {
	b := NewBudget(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "B",
		mustID(t, "33333333-3333-3333-3333-333333333333"), t0, t0)
	reset := time.Date(2024, 6, 20, 8, 0, 0, 0, time.UTC)
	b.StartFrom(reset, t1)
	want := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if !b.StartedAt.Equal(want) {
		t.Fatalf("startedAt=%v want %v", b.StartedAt, want)
	}
	if !b.UpdatedAt.Equal(t1) {
		t.Fatalf("StartFrom should bump updatedAt, got %v", b.UpdatedAt)
	}
}

func TestNewBudgetAccess_StartsPending(t *testing.T) {
	a := NewBudgetAccess(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "33333333-3333-3333-3333-333333333333"), BudgetRoleUser, t0)
	if a.IsAccepted {
		t.Fatal("new access must be pending (not accepted)")
	}
	if a.Role != BudgetRoleUser {
		t.Fatalf("role=%d want user", a.Role)
	}
}

func TestBudgetAccess_Accept_Idempotent(t *testing.T) {
	a := NewBudgetAccess(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "33333333-3333-3333-3333-333333333333"), BudgetRoleUser, t0)
	a.Accept(t1)
	if !a.IsAccepted || !a.UpdatedAt.Equal(t1) {
		t.Fatalf("accept: accepted=%v updatedAt=%v", a.IsAccepted, a.UpdatedAt)
	}
	// second accept is a no-op (no bump).
	a.Accept(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC))
	if !a.UpdatedAt.Equal(t1) {
		t.Fatalf("re-accept bumped updatedAt to %v", a.UpdatedAt)
	}
}

func TestBudgetAccess_UpdateRole_OnlyBumpsOnChange(t *testing.T) {
	a := NewBudgetAccess(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "33333333-3333-3333-3333-333333333333"), BudgetRoleUser, t0)
	a.UpdateRole(BudgetRoleUser, t1)
	if !a.UpdatedAt.Equal(t0) {
		t.Fatal("same-role update bumped updatedAt")
	}
	a.UpdateRole(BudgetRoleAdmin, t1)
	if a.Role != BudgetRoleAdmin || !a.UpdatedAt.Equal(t1) {
		t.Fatalf("role change: role=%d updatedAt=%v", a.Role, a.UpdatedAt)
	}
}

func TestBudgetElement_PositionUnset(t *testing.T) {
	e := NewBudgetElement(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "33333333-3333-3333-3333-333333333333"), ElementCategory, nil, nil, PositionUnset, t0)
	if !e.IsPositionUnset() {
		t.Fatal("position 0 should be unset")
	}
	e.UpdatePosition(5, t1)
	if e.IsPositionUnset() || e.Position != 5 || !e.UpdatedAt.Equal(t1) {
		t.Fatalf("after UpdatePosition: pos=%d unset=%v", e.Position, e.IsPositionUnset())
	}
}

func TestBudgetElement_UpdateCurrency_NilTransitions(t *testing.T) {
	cur := mustID(t, "33333333-3333-3333-3333-333333333333")
	e := NewBudgetElement(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "44444444-4444-4444-4444-444444444444"), ElementCategory, nil, nil, 1, t0)
	// nil -> nil: no bump.
	e.UpdateCurrency(nil, t1)
	if !e.UpdatedAt.Equal(t0) {
		t.Fatal("nil->nil currency bumped updatedAt")
	}
	// nil -> set: bump.
	e.UpdateCurrency(&cur, t1)
	if e.CurrencyID == nil || !e.CurrencyID.Equal(cur) || !e.UpdatedAt.Equal(t1) {
		t.Fatalf("nil->set currency failed: %v", e.CurrencyID)
	}
	// set -> same: no bump (reset updatedAt to detect).
	e2 := &BudgetElement{ID: e.ID, BudgetID: e.BudgetID, ExternalID: e.ExternalID, Type: e.Type, CurrencyID: &cur, Position: 1, CreatedAt: t0, UpdatedAt: t0}
	e2.UpdateCurrency(&cur, t1)
	if !e2.UpdatedAt.Equal(t0) {
		t.Fatal("set->same currency bumped updatedAt")
	}
}

func TestBudgetElement_UpdateFolder_NilTransitions(t *testing.T) {
	folder := mustID(t, "55555555-5555-5555-5555-555555555555")
	e := NewBudgetElement(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		mustID(t, "44444444-4444-4444-4444-444444444444"), ElementCategory, nil, nil, 1, t0)
	e.UpdateFolder(&folder, t1)
	if e.FolderID == nil || !e.FolderID.Equal(folder) || !e.UpdatedAt.Equal(t1) {
		t.Fatalf("move to folder failed: %v", e.FolderID)
	}
	// clear it.
	e.UpdateFolder(nil, time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC))
	if e.FolderID != nil {
		t.Fatalf("folder should be cleared, got %v", e.FolderID)
	}
}

func TestNewBudgetElementLimit_SnapsPeriod(t *testing.T) {
	l := NewBudgetElementLimit(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), vo.NewDecimal("200"), t0, t0)
	want := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	if !l.Period.Equal(want) {
		t.Fatalf("period=%v want %v (first of month)", l.Period, want)
	}
	if l.Amount.String() != "200" {
		t.Fatalf("amount=%q want 200", l.Amount.String())
	}
}

func TestBudgetElementLimit_UpdateAmount_OnlyBumpsOnChange(t *testing.T) {
	l := NewBudgetElementLimit(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), vo.NewDecimal("200"), t0, t0)
	// "200" == "200.00000000" -> no change.
	l.UpdateAmount(vo.NewDecimal("200.00000000"), t1)
	if !l.UpdatedAt.Equal(t0) {
		t.Fatal("equal-amount update bumped updatedAt")
	}
	l.UpdateAmount(vo.NewDecimal("250.50"), t1)
	if l.Amount.String() != "250.5" || !l.UpdatedAt.Equal(t1) {
		t.Fatalf("amount change: %q / %v", l.Amount.String(), l.UpdatedAt)
	}
}

func TestBudgetEnvelope_SetArchived_OnlyBumpsOnChange(t *testing.T) {
	e := NewBudgetEnvelope(mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"), "Travel", "plane", t0)
	if e.IsArchived {
		t.Fatal("new envelope must not be archived")
	}
	e.SetArchived(false, t1)
	if !e.UpdatedAt.Equal(t0) {
		t.Fatal("no-op archive bumped updatedAt")
	}
	e.SetArchived(true, t1)
	if !e.IsArchived || !e.UpdatedAt.Equal(t1) {
		t.Fatalf("archive: archived=%v updatedAt=%v", e.IsArchived, e.UpdatedAt)
	}
}

func TestFirstOfMonth(t *testing.T) {
	in := time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC)
	got := FirstOfMonth(in)
	want := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("FirstOfMonth=%v want %v", got, want)
	}
}

func TestFirstOfMonth_PreservesLocation(t *testing.T) {
	loc := time.FixedZone("X", 5*3600)
	in := time.Date(2024, 7, 15, 10, 0, 0, 0, loc)
	got := FirstOfMonth(in)
	if got.Location() != loc {
		t.Fatalf("location not preserved: %v", got.Location())
	}
}

func TestIdPtrEqual(t *testing.T) {
	a := mustID(t, "11111111-1111-1111-1111-111111111111")
	b := mustID(t, "22222222-2222-2222-2222-222222222222")
	if !idPtrEqual(nil, nil) {
		t.Error("nil==nil should be equal")
	}
	if idPtrEqual(&a, nil) || idPtrEqual(nil, &a) {
		t.Error("one nil should be unequal")
	}
	if !idPtrEqual(&a, &a) {
		t.Error("same id should be equal")
	}
	if idPtrEqual(&a, &b) {
		t.Error("different ids should be unequal")
	}
}

// asValidation reports whether err is a *errs.ValidationError, binding it.
func asValidation(err error, target **errs.ValidationError) bool {
	ve, ok := err.(*errs.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}
