package budget_test

import "testing"

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
	h.mustCreate(t, h.owner, "cat-food", "2026-06-01", "after the clone start")

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
