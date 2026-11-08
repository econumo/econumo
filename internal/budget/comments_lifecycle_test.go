package budget_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestResetBudget_DeletesComments(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "before the reset")

	h.reset(t, "2026-05-01")

	res := h.list(t, h.owner, "2026-05-01", "12")
	if len(res.Items) != 0 {
		t.Fatalf("items=%d want 0: reset re-anchors the start month, so kept comments would be orphans", len(res.Items))
	}
}

func TestCloneBudget_CopiesCommentsWithLimitsOnly(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-04-01", "before the clone start")
	after := h.mustCreate(t, h.owner, "cat-food", "2026-06-01", "after the clone start")
	// Backdated well outside the test's real wall-clock run: a regression that
	// stamps the copy with the clone's own `now` instead of preserving the
	// source's timestamps must fail, not pass by coincidence.
	wantTime := time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC)
	h.backdateComment(t, after.Item.Id, wantTime)
	wantStamp := wantTime.Format(datetime.Layout)

	bare := h.clone(t, "Bare copy", "2026-05-01", false)
	if got := h.listIn(t, bare, h.owner, "2026-04-01", "12"); len(got.Items) != 0 {
		t.Fatalf("items=%d want 0 without withLimits", len(got.Items))
	}

	full := h.clone(t, "Full copy", "2026-05-01", true)
	got := h.listIn(t, full, h.owner, "2026-04-01", "12")
	if len(got.Items) != 1 || got.Items[0].Comment != "after the clone start" {
		t.Fatalf("items=%+v want only the comment at or after the start month", got.Items)
	}
	if got.Items[0].Author.Id != h.owner.String() {
		t.Fatalf("author=%q want the original author preserved", got.Items[0].Author.Id)
	}
	if got.Items[0].Id == "" {
		t.Fatal("copied comment kept an empty id")
	}
	if got.Items[0].CreatedAt != wantStamp || got.Items[0].UpdatedAt != wantStamp {
		t.Fatalf("createdAt=%q updatedAt=%q want both %q (the source's, not the clone's now)",
			got.Items[0].CreatedAt, got.Items[0].UpdatedAt, wantStamp)
	}
}

func TestMergeCategories_RepointsComments(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "on the source")
	h.mustCreate(t, h.owner, "cat-transport", "2026-05-01", "on the target")

	h.mergeCategory(t, "cat-food", "cat-transport")

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 2 {
		t.Fatalf("items=%d want both threads on the target", len(res.Items))
	}
	for _, item := range res.Items {
		if item.ElementId != h.catTransport {
			t.Fatalf("elementId=%q want the merge target", item.ElementId)
		}
	}
}

// TestCloneBudget_RefusesAnIdAlreadyInUse covers a silent-corruption path: the
// copy's id comes from the client and the budget write is an upsert
// (ON CONFLICT (id) DO UPDATE), so an id that is already taken would rewrite
// that budget's own settings and then layer the source's folders, envelopes,
// elements and limits under it. A model reusing an id it already has - the
// source's own, or a retry of the same clone - must get a field error, and the
// existing budget must come back untouched.
func TestCloneBudget_RefusesAnIdAlreadyInUse(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "on the original")

	before, err := h.svc.GetBudgetList(h.ctx, h.owner)
	if err != nil {
		t.Fatalf("GetBudgetList: %v", err)
	}

	_, err = h.svc.CloneBudget(h.ctx, h.owner, model.CloneBudgetRequest{
		Id:    h.budgetID.String(),
		NewId: h.budgetID.String(), // the source's own id
		Name:  "Overwriting copy",
	})
	if err == nil {
		t.Fatal("CloneBudget onto an existing id succeeded; it must be refused")
	}
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v (%T), want a validation error naming newId", err, err)
	}
	// The code must say the id is taken, not that the field was blank: a model
	// told "should not be blank" about an id it did send would retry the same
	// one.
	named := false
	for _, f := range v.Fields {
		if f.Key == "newId" {
			named = true
			if f.Code != errs.CodeBudgetIDInUse {
				t.Fatalf("newId code = %q, want %q", f.Code, errs.CodeBudgetIDInUse)
			}
		}
	}
	if !named {
		t.Fatalf("validation fields = %+v, want one naming newId", v.Fields)
	}

	// Nothing was created, and the source kept its own name and comments.
	after, err := h.svc.GetBudgetList(h.ctx, h.owner)
	if err != nil {
		t.Fatalf("GetBudgetList: %v", err)
	}
	if len(after.Items) != len(before.Items) {
		t.Fatalf("budget count %d -> %d: the refused clone still wrote rows", len(before.Items), len(after.Items))
	}
	for i := range after.Items {
		if after.Items[i].Name != before.Items[i].Name {
			t.Fatalf("budget %d name %q -> %q: the refused clone overwrote it",
				i, before.Items[i].Name, after.Items[i].Name)
		}
	}
	if got := h.list(t, h.owner, "2026-05-01", "12"); len(got.Items) != 1 {
		t.Fatalf("comments on the source = %d, want the original 1 kept", len(got.Items))
	}
}

// TestCreateBudget_RefusesAnotherUsersBudgetID covers an IDOR on the REST
// create-budget path, where req.Id is the budget's primary key rather than an
// operation id (every other create mints the entity id server-side) and the
// write used to be an upsert. A caller who knows an existing budget's id - a
// participant whose access was later revoked still knows it - could rewrite
// that budget's name, currency, dates, archived flag and start day, and then
// have their own accounts seeded onto it as member accounts. MCP is unaffected
// because its handler mints the id itself.
func TestCreateBudget_RefusesAnotherUsersBudgetID(t *testing.T) {
	h := newCommentHarness(t)

	before, err := h.svc.GetBudgetList(h.ctx, h.owner)
	if err != nil {
		t.Fatalf("GetBudgetList: %v", err)
	}
	var victim *model.MetaResult
	for i := range before.Items {
		if before.Items[i].Id == h.budgetID.String() {
			victim = &before.Items[i]
		}
	}
	if victim == nil {
		t.Fatalf("the harness budget %s is not in the owner's list", h.budgetID)
	}

	// The attacker owns an account, so the only thing standing between them and
	// the victim's budget row is the id check.
	attackerAccount := h.f.Account(fixture.Account{UserID: h.stranger.String(), Name: "Attacker wallet"})

	_, err = h.svc.CreateBudget(h.ctx, h.stranger, model.CreateBudgetRequest{
		Id:   h.budgetID.String(), // another user's budget
		Name: "Seized",
		// Explicit, so the call does not stop at the attacker's missing default
		// currency option before it reaches the id check.
		CurrencyId: fixture.USD,
		AccountIds: []string{attackerAccount},
	})
	if err == nil {
		t.Fatal("CreateBudget onto another user's budget id succeeded; it must be refused")
	}
	v, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err = %v (%T), want a validation error naming id", err, err)
	}
	found := false
	for _, f := range v.Fields {
		if f.Key == "id" {
			found = true
			if f.Code != errs.CodeBudgetIDInUse {
				t.Fatalf("id code = %q, want %q", f.Code, errs.CodeBudgetIDInUse)
			}
		}
	}
	if !found {
		t.Fatalf("validation fields = %+v, want one naming id", v.Fields)
	}

	// The victim's budget is untouched: same name, and no attacker account
	// grafted on as a member.
	after, err := h.svc.GetBudgetList(h.ctx, h.owner)
	if err != nil {
		t.Fatalf("GetBudgetList: %v", err)
	}
	for i := range after.Items {
		if after.Items[i].Id != h.budgetID.String() {
			continue
		}
		if after.Items[i].Name != victim.Name {
			t.Fatalf("victim budget name %q -> %q: the refused create overwrote it",
				victim.Name, after.Items[i].Name)
		}
		if after.Items[i].OwnerUserId != victim.OwnerUserId {
			t.Fatalf("victim budget owner %q -> %q", victim.OwnerUserId, after.Items[i].OwnerUserId)
		}
		if after.Items[i].PeriodStartDay != victim.PeriodStartDay {
			t.Fatalf("victim budget periodStartDay %d -> %d", victim.PeriodStartDay, after.Items[i].PeriodStartDay)
		}
	}
	members, err := h.budgetRepo.MemberAccounts(h.ctx, h.budgetID)
	if err != nil {
		t.Fatalf("MemberAccounts: %v", err)
	}
	for _, m := range members {
		if m.AccountID.String() == attackerAccount {
			t.Fatal("the attacker's account was added to the victim's budget")
		}
	}
}
