package budget_test

import (
	"slices"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func sortedIDs(ids []vo.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	slices.Sort(out)
	return out
}

func sorted(ids ...vo.Id) []string { return sortedIDs(ids) }

func TestDebtBudget_RolesAndParticipants(t *testing.T) {
	h := newCommentHarness(t)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID.String(), ExternalID: vo.NewId().String(), Type: int(model.ElementEnvelope), CurrencyID: eur})
	cash := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Cash"})
	h.f.BudgetAccount(h.budgetID.String(), cash)

	participants := sorted(h.owner, h.admin, h.member)
	for name, tc := range map[string]struct {
		user    vo.Id
		canPlan bool
	}{
		"owner": {h.owner, true}, "admin": {h.admin, true}, "member": {h.member, true}, "guest": {h.guest, false},
	} {
		b, err := h.svc.DebtBudget(h.ctx, tc.user, h.budgetID)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if b.CanPlan != tc.canPlan {
			t.Errorf("%s: CanPlan = %v, want %v", name, b.CanPlan, tc.canPlan)
		}
		if got := sortedIDs(b.ParticipantIDs); !slices.Equal(got, participants) {
			t.Errorf("%s: participants = %v, want %v (no guest, no pending invite)", name, got, participants)
		}
		if !b.ID.Equal(h.budgetID) || b.CurrencyID.String() != fixture.USD || b.PeriodStartDay != model.MinPeriodStartDay || b.IsArchived {
			t.Errorf("%s: budget = %+v", name, b)
		}
		if got := sortedIDs(b.CurrencyIDs); !slices.Equal(got, sorted(vo.MustParseId(fixture.USD), vo.MustParseId(eur))) {
			t.Errorf("%s: currencies = %v, want the budget's and the element's", name, got)
		}
		if len(b.AccountIDs) != 1 || b.AccountIDs[0].String() != cash {
			t.Errorf("%s: accounts = %v, want the member account", name, b.AccountIDs)
		}
	}
	for name, user := range map[string]vo.Id{"pending": h.pending, "stranger": h.stranger} {
		if _, err := h.svc.DebtBudget(h.ctx, user, h.budgetID); !isNotFound(err) {
			t.Errorf("%s: err = %v, want not found", name, err)
		}
	}
	if _, err := h.svc.DebtBudget(h.ctx, h.owner, vo.NewId()); !isNotFound(err) {
		t.Errorf("unknown budget: err = %v, want not found", err)
	}

	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind("UPDATE budgets SET is_archived = 1 WHERE id = ?"), h.budgetID.String()); err != nil {
		t.Fatal(err)
	}
	if b, err := h.svc.DebtBudget(h.ctx, h.owner, h.budgetID); err != nil || !b.IsArchived || !b.CanPlan {
		t.Errorf("archived: %+v, %v; want archived, still the owner's to plan", b, err)
	}

	list, err := h.svc.DebtBudgets(h.ctx, h.guest)
	if err != nil || len(list) != 1 || !list[0].ID.Equal(h.budgetID) || list[0].CanPlan {
		t.Errorf("guest's list = %+v, %v; want the budget, read-only", list, err)
	}
	if list, err := h.svc.DebtBudgets(h.ctx, h.pending); err != nil || len(list) != 0 {
		t.Errorf("pending invite's list = %+v, %v; want none", list, err)
	}
}

func isNotFound(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

// The scope is the population the budget counts: a soft-deleted member keeps
// counting, so its payments keep counting on a debt too.
func TestDebtScope_IncludesSoftDeletedMembers(t *testing.T) {
	h := newCommentHarness(t)
	live := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Live"})
	gone := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Gone", Deleted: true})
	h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Outside"})
	h.f.BudgetAccount(h.budgetID.String(), live)
	h.f.BudgetAccount(h.budgetID.String(), gone)

	scope, err := h.svc.DebtScope(h.ctx, h.budgetID)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.BudgetID.Equal(h.budgetID) || !slices.Equal(sortedIDs(scope.AccountIDs), sorted(vo.MustParseId(live), vo.MustParseId(gone))) {
		t.Errorf("scope = %+v, want both members, the deleted one included", scope)
	}
	b, err := h.svc.DebtBudget(h.ctx, h.owner, h.budgetID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedIDs(b.AccountIDs), sortedIDs(scope.AccountIDs)) {
		t.Errorf("access accounts = %v, want the scope's %v", b.AccountIDs, scope.AccountIDs)
	}
}

func TestDebtRuleOptions_CallerFirstActiveExpenseOnly(t *testing.T) {
	h := newCommentHarness(t)
	own := h.owner.String()
	loan := h.f.Category(fixture.Category{UserID: own, Name: "Loan", Icon: "home"})
	h.f.Category(fixture.Category{UserID: own, Name: "Salary", Type: 1})
	h.f.Category(fixture.Category{UserID: own, Name: "Old loan", Archived: true})
	bank := h.f.Payee(fixture.Payee{UserID: own, Name: "Bank"})
	h.f.Payee(fixture.Payee{UserID: own, Name: "Closed bank", Archived: true})
	home := h.f.Tag(fixture.Tag{UserID: own, Name: "Home"})
	h.f.Tag(fixture.Tag{UserID: own, Name: "Old", Archived: true})
	car := h.f.Label(fixture.Label{UserID: own, Name: "Car", Icon: "car"})
	h.f.Label(fixture.Label{UserID: own, Name: "Gone", Archived: true})
	memberLoan := h.f.Category(fixture.Category{UserID: h.member.String(), Name: "Student loan"})
	h.f.Category(fixture.Category{UserID: h.guest.String(), Name: "Guest loan"})

	res, err := h.svc.DebtRuleOptions(h.ctx, h.member, h.budgetID)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, p := range res.Participants {
		order = append(order, p.Name)
	}
	// The member asks: they come first, then the others by name; the guest
	// and the pending invite are not participants.
	if !slices.Equal(order, []string{"Member", "Admin", "Owner"}) {
		t.Fatalf("participants = %v, want Member, Admin, Owner", order)
	}
	if m := res.Participants[0]; m.UserId != h.member.String() || len(m.Categories) != 1 || m.Categories[0].Id != memberLoan {
		t.Errorf("member = %+v, want their one category", m)
	}
	o := res.Participants[2]
	if len(o.Categories) != 1 || o.Categories[0].Id != loan || o.Categories[0].Name != "Loan" || o.Categories[0].Icon != "home" {
		t.Errorf("owner's categories = %+v, want only the active expense one", o.Categories)
	}
	if len(o.Payees) != 1 || o.Payees[0].Id != bank {
		t.Errorf("owner's payees = %+v, want only the active one", o.Payees)
	}
	if len(o.Tags) != 1 || o.Tags[0].Id != home {
		t.Errorf("owner's tags = %+v, want only the active one", o.Tags)
	}
	if len(o.Labels) != 1 || o.Labels[0].Id != car || o.Labels[0].Icon != "car" {
		t.Errorf("owner's labels = %+v, want only the active one", o.Labels)
	}
	if a := res.Participants[1]; a.Categories == nil || a.Payees == nil || a.Tags == nil || a.Labels == nil {
		t.Errorf("admin = %+v, want empty lists, not null", a)
	}
}
