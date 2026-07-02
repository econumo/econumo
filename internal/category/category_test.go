package category

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	v, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %q: %v", s, err)
	}
	return v
}

var (
	tk0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	tk1 = time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
)

func TestType_AliasAndInt(t *testing.T) {
	if TypeExpense != 0 || TypeIncome != 1 {
		t.Fatalf("type values drifted: expense=%d income=%d", TypeExpense, TypeIncome)
	}
	if TypeExpense.Alias() != "expense" || TypeIncome.Alias() != "income" {
		t.Fatalf("alias: expense=%q income=%q", TypeExpense.Alias(), TypeIncome.Alias())
	}
	if TypeExpense.Int16() != 0 || TypeIncome.Int16() != 1 {
		t.Fatal("Int16 mismatch")
	}
}

func newCat(t *testing.T, typ Type) *Category {
	return NewCategory(
		mustID(t, "11111111-1111-1111-1111-111111111111"),
		mustID(t, "22222222-2222-2222-2222-222222222222"),
		"Food", typ, "cart", tk0)
}

func TestNewCategory_Defaults(t *testing.T) {
	c := newCat(t, TypeExpense)
	if c.IsArchived() {
		t.Fatal("new category must not be archived")
	}
	if c.Type() != TypeExpense || c.Name() != "Food" || c.Icon() != "cart" {
		t.Fatalf("category fields wrong: %+v", c)
	}
}

func TestCategory_Archive_Unarchive_OnlyBumpOnChange(t *testing.T) {
	c := newCat(t, TypeExpense)
	c.Unarchive(tk1) // already not archived -> no-op
	if !c.UpdatedAt().Equal(tk0) {
		t.Fatal("no-op unarchive bumped updatedAt")
	}
	c.Archive(tk1)
	if !c.IsArchived() || !c.UpdatedAt().Equal(tk1) {
		t.Fatalf("archive: archived=%v updatedAt=%v", c.IsArchived(), c.UpdatedAt())
	}
	c.Archive(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)) // no-op
	if !c.UpdatedAt().Equal(tk1) {
		t.Fatal("re-archive bumped updatedAt")
	}
}

func TestCategory_UpdateName_OnlyBumpsOnChange(t *testing.T) {
	c := newCat(t, TypeExpense)
	c.UpdateName("Food", tk1)
	if !c.UpdatedAt().Equal(tk0) {
		t.Fatal("same-name update bumped updatedAt")
	}
	c.UpdateName("Dining", tk1)
	if c.Name() != "Dining" || !c.UpdatedAt().Equal(tk1) {
		t.Fatalf("rename: %q / %v", c.Name(), c.UpdatedAt())
	}
}
