package budget_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
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
	named := false
	for _, f := range v.Fields {
		if f.Key == "newId" {
			named = true
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
