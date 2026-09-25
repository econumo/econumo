package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

// The savings role belongs to the budget membership, not to the account: the
// same account can be savings in one budget and everyday in another.

const (
	savingsFlagBudgetB = "bbbb2222-0000-7000-8000-0000000005b0"
	savingsFlagCopyID  = "bbbb2222-0000-7000-8000-0000000005c0"
)

func (h *harness) savingsBudgetOf(t *testing.T, tok, budgetID, date string) savingsBudgetView {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID+"&date="+date, tok, nil)
	return mustUnmarshal[savingsBudgetView](t, env.Data)
}

func TestSavingsFlag_PerBudget(t *testing.T) {
	h, tok, _ := newSavingsBudget(t) // budget A: S1 and S2 flagged
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": savingsFlagBudgetB, "name": "Budget B", "currencyId": usdID, "startDate": "2026-06-01",
		"accountIds": []string{accountID, savingsUSDID, savingsEURID},
	})
	flagSavings(t, h.tdb, savingsFlagBudgetB, savingsUSDID, true) // S2 stays everyday in B
	// S2 -> S1: savings-to-savings in A (not counted), everyday-to-savings in B.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, Type: 2, AccountID: savingsEURID, AccountRecipientID: savingsUSDID,
		Amount: "20", AmountRecipient: "22", SpentAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)})

	a := savingsByID(h.savingsBudgetOf(t, tok, budgetID1, "2026-08-15").Item.Structure.Savings)
	if len(a) != 2 {
		t.Fatalf("A savings = %+v, want S1 and S2", a)
	}
	if a[savingsUSDID].Spent != "300" {
		t.Errorf("A S1 spent = %s, want 300 (the S2 transfer is savings-to-savings)", a[savingsUSDID].Spent)
	}

	bRows := h.savingsBudgetOf(t, tok, savingsFlagBudgetB, "2026-08-15").Item.Structure.Savings
	b := savingsByID(bRows)
	if _, ok := b[savingsEURID]; ok || len(bRows) != 1 {
		t.Fatalf("B savings = %+v, want only S1 (S2 is not flagged in B)", bRows)
	}
	if b[savingsUSDID].Spent != "322" {
		t.Errorf("B S1 spent = %s, want 322 (S2 is an everyday account in B)", b[savingsUSDID].Spent)
	}
}

func TestSavingsFlag_CloneCopiesFlag(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	flagSavings(t, h.tdb, budgetID1, savingsEURID, false)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": budgetID1, "newId": savingsFlagCopyID, "name": "Copy", "withLimits": true})

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, savingsFlagCopyID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("copy members = %d (err %v), want all 3", n, err)
	}
	rows := h.savingsBudgetOf(t, tok, savingsFlagCopyID, "2026-08-15").Item.Structure.Savings
	if len(rows) != 1 || rows[0].Id != savingsUSDID || rows[0].Budgeted != "400" {
		t.Fatalf("copy savings = %+v, want only the flagged S1 with its plan", rows)
	}
}

type flagsView struct {
	Item struct {
		Meta struct {
			Name string `json:"name"`
		} `json:"meta"`
		Filters struct {
			Accounts []struct {
				Id        string `json:"id"`
				Removable bool   `json:"removable"`
				IsSavings bool   `json:"isSavings"`
			} `json:"accounts"`
		} `json:"filters"`
		Structure struct {
			Savings []savingsElementView `json:"savings"`
		} `json:"structure"`
	} `json:"item"`
}

func (h *harness) flagsOf(t *testing.T, tok, budgetID string) flagsView {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID+"&date=2026-08-15", tok, nil)
	return mustUnmarshal[flagsView](t, env.Data)
}

// filterFlags is the requester's own members as get-budget reports them.
func (v flagsView) filterFlags() map[string]bool {
	out := map[string]bool{}
	for _, a := range v.Item.Filters.Accounts {
		out[a.Id] = a.IsSavings
	}
	return out
}

// memberFlags reads budgets_accounts directly: member -> is_savings, for every
// participant's accounts.
func memberFlags(t *testing.T, h *harness, budgetID string) map[string]bool {
	t.Helper()
	rows, err := h.db.Query(`SELECT account_id, is_savings FROM budgets_accounts WHERE budget_id = ?`, budgetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		var on bool
		if err := rows.Scan(&id, &on); err != nil {
			t.Fatal(err)
		}
		out[id] = on
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// savingsData counts the savings element, its limits and its comments for one
// account in one budget.
func savingsData(t *testing.T, h *harness, budgetID, accountID string) (elements, limits, comments int) {
	t.Helper()
	q := func(sql string) int {
		var n int
		if err := h.db.QueryRow(sql, budgetID, accountID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	elements = q(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ? AND type = 5`)
	limits = q(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id
		WHERE e.budget_id = ? AND e.external_id = ? AND e.type = 5`)
	comments = q(`SELECT COUNT(*) FROM budgets_elements_comments c JOIN budgets_elements e ON e.id = c.element_id
		WHERE e.budget_id = ? AND e.external_id = ? AND e.type = 5`)
	return
}

func (h *harness) comment(t *testing.T, tok, budgetID, elementID, id string) {
	t.Helper()
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-comment", tok, map[string]any{
		"id": id, "budgetId": budgetID, "elementId": elementID, "period": "2026-08-01", "comment": "rainy day",
	})
}

func wantFieldError(t *testing.T, label string, st int, env envelope, field string) {
	t.Helper()
	if st != http.StatusBadRequest || len(env.errorsMap()[field]) == 0 {
		t.Fatalf("%s: status=%d body=%s, want 400 on field %s", label, st, env.raw, field)
	}
}

func sameFlags(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: flags = %v, want %v", label, got, want)
	}
	for id, on := range want {
		if v, ok := got[id]; !ok || v != on {
			t.Fatalf("%s: flags = %v, want %v", label, got, want)
		}
	}
}

const (
	flagCommentID1 = "c0c01111-0000-7000-8000-0000000005d1"
	flagCommentID2 = "c0c01111-0000-7000-8000-0000000005d2"
	flagPartnerAcc = "aaaa2222-0000-7000-8000-0000000005e1"
)

func TestSavingsFlag_CreateBudget(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.Account(fixture.Account{ID: savingsUSDID, UserID: seedUserID, CurrencyID: usdID, Name: "Rainy day"})
	h.f.Account(fixture.Account{ID: savingsEURID, UserID: seedUserID, CurrencyID: usdID, Name: "Pot"})

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01",
		"accountIds": []string{accountID, savingsUSDID}, "savingsAccountIds": []string{savingsEURID},
	})
	wantFieldError(t, "savings id outside accountIds", st, env, "savingsAccountIds")
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets WHERE id = ?`, budgetID1).Scan(&n); err != nil || n != 0 {
		t.Fatalf("budgets = %d (err %v), want none after a refused create", n, err)
	}

	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01",
		"accountIds": []string{accountID, savingsUSDID, savingsEURID}, "savingsAccountIds": []string{savingsUSDID},
	})
	v := h.flagsOf(t, tok, budgetID1)
	sameFlags(t, "filters", v.filterFlags(), map[string]bool{accountID: false, savingsUSDID: true, savingsEURID: false})
	if rows := v.Item.Structure.Savings; len(rows) != 1 || rows[0].Id != savingsUSDID {
		t.Fatalf("savings = %+v, want only %s", rows, savingsUSDID)
	}
	if el, _, _ := savingsData(t, h, budgetID1, savingsUSDID); el != 1 {
		t.Fatalf("savings elements for S1 = %d, want 1 (synced at create)", el)
	}
}

func TestSavingsFlag_UpdateBudgetReplaceSet(t *testing.T) {
	h, tok, _ := newSavingsBudget(t) // S1, S2 flagged with plans; Cash everyday
	h.f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID, Name: "Spare"})
	h.f.User(fixture.User{ID: otherUserID, Email: "flag-other@example.test", Name: "Other"})
	h.f.Account(fixture.Account{ID: otherAccountID, UserID: otherUserID, CurrencyID: usdID, Name: "Theirs"})
	update := func(extra map[string]any) (int, envelope) {
		body := map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID}
		for k, v := range extra {
			body[k] = v
		}
		return h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, body)
	}

	// Flag Cash on; S1 and S2 stay.
	if st, env := update(map[string]any{"savingsAccountIds": []string{savingsUSDID, savingsEURID, accountID}}); st != http.StatusOK {
		t.Fatalf("flag cash: %d %s", st, env.raw)
	}
	all := map[string]bool{accountID: true, savingsUSDID: true, savingsEURID: true}
	sameFlags(t, "after flagging cash", h.flagsOf(t, tok, budgetID1).filterFlags(), all)
	if el, _, _ := savingsData(t, h, budgetID1, accountID); el != 1 {
		t.Fatalf("cash savings elements = %d, want 1 in the same request", el)
	}

	// Absent leaves every flag alone.
	if st, env := update(map[string]any{"name": "Renamed"}); st != http.StatusOK {
		t.Fatalf("rename: %d %s", st, env.raw)
	}
	sameFlags(t, "after rename", memberFlags(t, h, budgetID1), all)

	// Not the caller's member after accountIds applies -> refused, nothing changed.
	for _, c := range []struct {
		label string
		extra map[string]any
	}{
		{"owned non-member", map[string]any{"savingsAccountIds": []string{accountID2}}},
		{"someone else's account", map[string]any{"savingsAccountIds": []string{otherAccountID}}},
		{"dropped by accountIds", map[string]any{
			"accountIds":        []string{savingsUSDID, savingsEURID},
			"savingsAccountIds": []string{savingsUSDID, savingsEURID, accountID},
		}},
		{"garbage id", map[string]any{"savingsAccountIds": []string{"nope"}}},
	} {
		c.extra["name"] = "Changed"
		st, env := update(c.extra)
		wantFieldError(t, c.label, st, env, "savingsAccountIds")
		v := h.flagsOf(t, tok, budgetID1)
		if v.Item.Meta.Name != "Renamed" {
			t.Fatalf("%s: name = %q, want the refused request to write nothing", c.label, v.Item.Meta.Name)
		}
		sameFlags(t, c.label, memberFlags(t, h, budgetID1), all)
	}

	// Adding a member and flagging it in one request works: the set is checked
	// after accountIds applies.
	if st, env := update(map[string]any{
		"accountIds":        []string{accountID, savingsUSDID, savingsEURID, accountID2},
		"savingsAccountIds": []string{savingsUSDID, savingsEURID, accountID, accountID2},
	}); st != http.StatusOK {
		t.Fatalf("add+flag: %d %s", st, env.raw)
	}
	all[accountID2] = true
	sameFlags(t, "after add+flag", memberFlags(t, h, budgetID1), all)

	// An unplanned, uncommented savings member turns off without confirmation,
	// and its empty element goes in the same request.
	if st, env := update(map[string]any{"savingsAccountIds": []string{savingsUSDID, savingsEURID}}); st != http.StatusOK {
		t.Fatalf("unflag empty members: %d %s", st, env.raw)
	}
	sameFlags(t, "after unflag", memberFlags(t, h, budgetID1),
		map[string]bool{accountID: false, savingsUSDID: true, savingsEURID: true, accountID2: false})
	for _, id := range []string{accountID, accountID2} {
		if el, _, _ := savingsData(t, h, budgetID1, id); el != 0 {
			t.Fatalf("%s savings elements = %d, want 0 after unflag", id, el)
		}
	}
}

// Review Focus 3: the replace-set is over the CALLER's own members only.
func TestSavingsFlag_ParticipantNeverTouchesOwnerFlags(t *testing.T) {
	h, _, _ := newSavingsBudget(t)
	h.f.User(fixture.User{ID: otherUserID, Email: "flag-partner@example.test", Name: "Partner"})
	h.f.BudgetAccess(budgetID1, otherUserID, 1, true)
	h.f.Account(fixture.Account{ID: flagPartnerAcc, UserID: otherUserID, CurrencyID: usdID, Name: "Partner pot"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/add-account", otherUserID,
		map[string]any{"id": budgetID1, "accountId": flagPartnerAcc, "isSavings": true})
	owner := map[string]bool{accountID: false, savingsUSDID: true, savingsEURID: true}
	check := func(label string, partner bool) {
		t.Helper()
		want := map[string]bool{flagPartnerAcc: partner}
		for k, v := range owner {
			want[k] = v
		}
		sameFlags(t, label, memberFlags(t, h, budgetID1), want)
	}
	check("partner add-account", true)

	update := func(extra map[string]any) (int, envelope) {
		body := map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID}
		for k, v := range extra {
			body[k] = v
		}
		return h.do(t, http.MethodPost, "/api/v1/budget/update-budget", otherUserID, body)
	}
	if st, env := update(nil); st != http.StatusOK {
		t.Fatalf("partner update without savings: %d %s", st, env.raw)
	}
	check("partner update without savingsAccountIds", true)
	if st, env := update(map[string]any{"savingsAccountIds": []string{}}); st != http.StatusOK {
		t.Fatalf("partner update with empty savings: %d %s", st, env.raw)
	}
	check("partner clears their own flags", false)
	st, env := update(map[string]any{"savingsAccountIds": []string{savingsUSDID, flagPartnerAcc}})
	wantFieldError(t, "partner names the owner's account", st, env, "savingsAccountIds")
	check("after refused partner update", false)
	if v := h.flagsOf(t, otherUserID, budgetID1).filterFlags(); len(v) != 1 {
		t.Fatalf("partner filters = %v, want only their own account", v)
	}
}

// Review Focus 1, 2 and 4: the confirmation guard on update-budget.
func TestSavingsFlag_UpdateBudgetGuard(t *testing.T) {
	h, tok, _ := newSavingsBudget(t) // S1 plan 400, S2 plan 50
	h.comment(t, tok, budgetID1, savingsUSDID, flagCommentID1)
	update := func(extra map[string]any) (int, envelope) {
		body := map[string]any{"id": budgetID1, "name": "Renamed", "currencyId": usdID}
		for k, v := range extra {
			body[k] = v
		}
		return h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, body)
	}
	unchanged := func(label string) {
		t.Helper()
		v := h.flagsOf(t, tok, budgetID1)
		if v.Item.Meta.Name != "Budget" {
			t.Fatalf("%s: name = %q, want the refused rename not written", label, v.Item.Meta.Name)
		}
		sameFlags(t, label, memberFlags(t, h, budgetID1), map[string]bool{accountID: false, savingsUSDID: true, savingsEURID: true})
		if el, lim, com := savingsData(t, h, budgetID1, savingsUSDID); el != 1 || lim != 1 || com != 1 {
			t.Fatalf("%s: S1 element/limits/comments = %d/%d/%d, want 1/1/1", label, el, lim, com)
		}
	}

	st, env := update(map[string]any{"savingsAccountIds": []string{savingsEURID}})
	wantFieldError(t, "flag off unconfirmed", st, env, "confirmSavingsRemoval")
	unchanged("flag off unconfirmed")

	st, env = update(map[string]any{"accountIds": []string{accountID, savingsEURID}})
	wantFieldError(t, "membership drop unconfirmed", st, env, "confirmSavingsRemoval")
	unchanged("membership drop unconfirmed")

	st, env = update(map[string]any{"accountIds": []string{accountID, savingsEURID}, "confirmSavingsRemoval": true})
	if st != http.StatusOK {
		t.Fatalf("confirmed drop: %d %s", st, env.raw)
	}
	sameFlags(t, "confirmed drop", memberFlags(t, h, budgetID1), map[string]bool{accountID: false, savingsEURID: true})
	if el, lim, com := savingsData(t, h, budgetID1, savingsUSDID); el != 0 || lim != 0 || com != 0 {
		t.Fatalf("S1 element/limits/comments = %d/%d/%d after confirmed drop, want all gone", el, lim, com)
	}

	// A comment alone is guarded too.
	h.setLimit(t, tok, savingsEURID, "2026-08-01", nil)
	h.comment(t, tok, budgetID1, savingsEURID, flagCommentID2)
	st, env = update(map[string]any{"savingsAccountIds": []string{}})
	wantFieldError(t, "comment-only unconfirmed", st, env, "confirmSavingsRemoval")
	if el, lim, com := savingsData(t, h, budgetID1, savingsEURID); el != 1 || lim != 0 || com != 1 {
		t.Fatalf("S2 element/limits/comments = %d/%d/%d, want 1/0/1", el, lim, com)
	}
	if st, env = update(map[string]any{"savingsAccountIds": []string{}, "confirmSavingsRemoval": true}); st != http.StatusOK {
		t.Fatalf("confirmed flag off: %d %s", st, env.raw)
	}
	if el, _, com := savingsData(t, h, budgetID1, savingsEURID); el != 0 || com != 0 {
		t.Fatalf("S2 element/comments = %d/%d after confirmed flag off, want 0/0", el, com)
	}
	if got := memberFlags(t, h, budgetID1)[savingsEURID]; got {
		t.Fatal("S2 still flagged after confirmed flag off")
	}
}

func TestSavingsFlag_AddAndRemoveAccount(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID, Name: "Spare"})
	add := func(body map[string]any) (int, envelope) {
		body["id"] = budgetID1
		return h.do(t, http.MethodPost, "/api/v1/budget/add-account", tok, body)
	}
	remove := func(body map[string]any) (int, envelope) {
		body["id"] = budgetID1
		return h.do(t, http.MethodPost, "/api/v1/budget/remove-account", tok, body)
	}

	if st, env := add(map[string]any{"accountId": accountID2, "isSavings": true}); st != http.StatusOK {
		t.Fatalf("add new savings member: %d %s", st, env.raw)
	}
	if el, _, _ := savingsData(t, h, budgetID1, accountID2); el != 1 {
		t.Fatalf("new savings member elements = %d, want 1", el)
	}
	if st, env := add(map[string]any{"accountId": accountID, "isSavings": true}); st != http.StatusOK {
		t.Fatalf("toggle cash on: %d %s", st, env.raw)
	}
	if st, env := add(map[string]any{"accountId": accountID}); st != http.StatusOK {
		t.Fatalf("re-add cash without flag: %d %s", st, env.raw)
	}
	flags := map[string]bool{accountID: true, accountID2: true, savingsUSDID: true, savingsEURID: true}
	sameFlags(t, "after add-account", memberFlags(t, h, budgetID1), flags)
	if st, env := add(map[string]any{"accountId": accountID, "isSavings": false}); st != http.StatusOK {
		t.Fatalf("toggle unplanned cash off: %d %s", st, env.raw)
	}
	flags[accountID] = false
	if el, _, _ := savingsData(t, h, budgetID1, accountID); el != 0 {
		t.Fatalf("cash savings elements = %d after toggle off, want 0", el)
	}

	st, env := add(map[string]any{"accountId": savingsUSDID, "isSavings": false})
	wantFieldError(t, "add-account flag off unconfirmed", st, env, "confirmSavingsRemoval")
	sameFlags(t, "after refused toggle", memberFlags(t, h, budgetID1), flags)
	if st, env := add(map[string]any{"accountId": savingsUSDID, "isSavings": false, "confirmSavingsRemoval": true}); st != http.StatusOK {
		t.Fatalf("confirmed toggle off: %d %s", st, env.raw)
	}
	flags[savingsUSDID] = false
	sameFlags(t, "after confirmed toggle", memberFlags(t, h, budgetID1), flags)
	if el, lim, _ := savingsData(t, h, budgetID1, savingsUSDID); el != 0 || lim != 0 {
		t.Fatalf("S1 element/limits = %d/%d after confirmed toggle, want 0/0", el, lim)
	}

	st, env = remove(map[string]any{"accountId": savingsEURID})
	wantFieldError(t, "remove planned savings unconfirmed", st, env, "confirmSavingsRemoval")
	sameFlags(t, "after refused remove", memberFlags(t, h, budgetID1), flags)
	if st, env := remove(map[string]any{"accountId": savingsEURID, "confirmSavingsRemoval": true}); st != http.StatusOK {
		t.Fatalf("confirmed remove: %d %s", st, env.raw)
	}
	delete(flags, savingsEURID)
	sameFlags(t, "after confirmed remove", memberFlags(t, h, budgetID1), flags)
	if el, lim, _ := savingsData(t, h, budgetID1, savingsEURID); el != 0 || lim != 0 {
		t.Fatalf("S2 element/limits = %d/%d after confirmed remove, want 0/0", el, lim)
	}

	// An unplanned savings member leaves without confirmation.
	if st, env := remove(map[string]any{"accountId": accountID2}); st != http.StatusOK {
		t.Fatalf("remove unplanned savings member: %d %s", st, env.raw)
	}
	if el, _, _ := savingsData(t, h, budgetID1, accountID2); el != 0 {
		t.Fatalf("removed member elements = %d, want 0", el)
	}
}

func TestSavingsFlag_ArchivedBudgetRefusesFlagWrites(t *testing.T) {
	h, tok, _ := newSavingsBudget(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1})
	for _, c := range []struct {
		label, path string
		body        map[string]any
	}{
		{"add-account", "/api/v1/budget/add-account", map[string]any{"id": budgetID1, "accountId": accountID, "isSavings": true}},
		{"update-budget", "/api/v1/budget/update-budget", map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "savingsAccountIds": []string{}, "confirmSavingsRemoval": true}},
		{"remove-account", "/api/v1/budget/remove-account", map[string]any{"id": budgetID1, "accountId": savingsUSDID, "confirmSavingsRemoval": true}},
	} {
		st, env := h.do(t, http.MethodPost, c.path, tok, c.body)
		if st != http.StatusForbidden || !strings.Contains(string(env.raw), "archived") {
			t.Fatalf("%s: status=%d body=%s, want 403 budget.archived", c.label, st, env.raw)
		}
	}
	sameFlags(t, "archived", memberFlags(t, h, budgetID1), map[string]bool{accountID: false, savingsUSDID: true, savingsEURID: true})
}
