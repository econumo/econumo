package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// txListView is the slice of get-transaction-list these tests care about.
type txListView struct {
	Items []struct {
		Id     string `json:"id"`
		Amount string `json:"amount"`
	} `json:"items"`
}

// A category listed as a child of a tag displays the tag-and-category
// intersection. Its drill-down must return exactly those transactions. Sending
// categoryId alone selects the "t.tag_id IS NULL" branch, which returns the
// complement — the reported bug.
func TestTagChildDrilldown_ReturnsTheIntersection(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	// Same category, same period: one tagged, one not.
	taggedID := f.Transaction(fixture.Transaction{
		ID: "eeee2222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, TagID: tagID, Type: 0, Amount: "42.00", SpentAt: "2024-04-10 00:00:00",
	})
	untaggedID := f.Transaction(fixture.Transaction{
		ID: "eeee2222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "7.00", SpentAt: "2024-04-11 00:00:00",
	})

	base := "/api/v1/budget/get-transaction-list?budgetId=" + budgetID1 + "&periodStart=2024-04-01"

	// The tag child: tagId AND categoryId together.
	st, b := h.do(t, http.MethodGet, base+"&tagId="+tagID+"&categoryId="+catID, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-transaction-list=%d body=%s", st, b.raw)
	}
	got := mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 {
		t.Fatalf("tag child drill-down must return only the tagged transaction, got %d: %s", len(got.Items), b.Data)
	}
	if got.Items[0].Id != taggedID {
		t.Errorf("tag child drill-down returned %q, want the tagged transaction %q", got.Items[0].Id, taggedID)
	}
	if vo.NewDecimal(got.Items[0].Amount).String() != vo.NewDecimal("42.00").String() {
		t.Errorf("tag child drill-down amount mismatch: got %q, want the seeded 42.00", got.Items[0].Amount)
	}

	// The standalone category row shows the untagged bucket, and its drill-down
	// must keep matching that. This is the branch the tag child wrongly used.
	_, b = h.do(t, http.MethodGet, base+"&categoryId="+catID, tok, nil)
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != untaggedID {
		t.Errorf("category-only drill-down must return only the untagged transaction %q; got %s", untaggedID, b.Data)
	}

	// The tag row itself shows everything tagged, across categories.
	_, b = h.do(t, http.MethodGet, base+"&tagId="+tagID, tok, nil)
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != taggedID {
		t.Errorf("tag drill-down must return the tagged transaction %q; got %s", taggedID, b.Data)
	}
}
