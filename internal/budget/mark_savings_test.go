package budget_test

import (
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestMarkSavingsMembers_FlagsOnlyOwnMembers(t *testing.T) {
	h := newNetWorthHarness(t)
	sav := h.f.Account(fixture.Account{UserID: h.user, Name: "Deposit"})
	outside := h.f.Account(fixture.Account{UserID: h.user, Name: "Not a member"})
	other := h.f.User(fixture.User{Name: "Other", Email: "other@networth.test"})
	theirs := h.f.Account(fixture.Account{UserID: other, Name: "Theirs"})
	b := h.budget(t, "2026-01-01 00:00:00", h.acct, sav, theirs)

	ids := []vo.Id{vo.MustParseId(sav), vo.MustParseId(outside), vo.MustParseId(theirs)}
	if err := h.svc.MarkSavingsMembers(h.ctx, vo.MustParseId(h.user), vo.MustParseId(b), ids); err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	rows, err := h.tdb.Raw.QueryContext(h.ctx, h.tdb.Rebind("SELECT account_id, is_savings FROM budgets_accounts WHERE budget_id = ?"), b)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var on bool
		if err := rows.Scan(&id, &on); err != nil {
			t.Fatal(err)
		}
		flags[id] = on
	}
	if !flags[sav] || flags[h.acct] || flags[theirs] {
		t.Fatalf("flags = %v", flags)
	}
	if _, member := flags[outside]; member {
		t.Fatal("a non-member must not be added")
	}
	// Marking again is a no-op, and a budget the caller may not touch changes nothing.
	if err := h.svc.MarkSavingsMembers(h.ctx, vo.MustParseId(h.user), vo.MustParseId(b), ids[:1]); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.MarkSavingsMembers(h.ctx, vo.MustParseId(other), vo.MustParseId(b), []vo.Id{vo.MustParseId(theirs)}); err == nil {
		t.Fatal("a participant without update rights must be refused")
	}
}
