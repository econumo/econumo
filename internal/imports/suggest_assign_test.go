package imports_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (h *harness) archivedPayee(t *testing.T, owner, name string) string {
	t.Helper()
	id := h.f.Payee(fixture.Payee{UserID: owner, Name: name, Archived: true})
	o := vo.MustParseId(owner)
	h.entities.payees[o] = append(h.entities.payees[o], model.ImportNamed{ID: id, Name: name, OwnerID: owner, Archived: true})
	return id
}

// assignSetup is a SimpleFIN source mapped to userA's account, with the AI
// job wired and both switches on.
func assignSetup(t *testing.T) (*harness, *fakeProvider, *fakeCompleter, fakeFacts) {
	t.Helper()
	h, p := bankHarness(t)
	c := &fakeCompleter{}
	facts := fakeFacts{userA: {language: "en"}}
	h.wireSuggest(c, facts)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	return h, p, c, facts
}

func syncRows(t *testing.T, h *harness, p *fakeProvider, txs ...model.ExternalTransaction) {
	t.Helper()
	p.txs = txs
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}
}

func payeeOwners(t *testing.T, h *harness, name string) []string {
	t.Helper()
	rows, err := h.db.Raw.QueryContext(context.Background(), h.db.Rebind(`SELECT user_id FROM payees WHERE name = ? ORDER BY user_id`), name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func TestAssignWritesEmptyFieldsOnly(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	ruled := h.category(t, userA, "Coffee")
	other := h.category(t, userA, "Restaurants")
	py := h.payee(t, userA, "Blue Bottle")
	// the rule set the category, so only the payee is empty
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: ruled})
	c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","merchants":["Blue Bottle"],"categoryId":"` + other + `"}]}`
	ingest(t, h, tap)
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if len(links) != 1 {
		t.Fatalf("links = %+v", links)
	}
	tx := txOf(t, h, links[0].ID)
	if !idIs(tx.CategoryID, ruled) || !idIs(tx.PayeeID, py) {
		t.Fatalf("transaction = %+v, want the rule's category and the AI's payee", tx)
	}
	l, _ := h.repo.GetLink(context.Background(), links[0].ID)
	if !idIs(l.AppliedCategoryID, ruled) || !idIs(l.AppliedPayeeID, py) || l.AppliedRuleID == nil {
		t.Fatalf("applied = category %v payee %v rule %v", l.AppliedCategoryID, l.AppliedPayeeID, l.AppliedRuleID)
	}
	got := assignRecords(t, h, userA)
	if len(got) != 1 || got[0].LinkID != links[0].ID || got[0].CategoryID != nil || !idIs(got[0].PayeeID, py) ||
		got[0].Status != model.ImportTransactionSuggestionStatusFilled || got[0].Type != model.TransactionTypeExpense {
		t.Fatalf("records = %+v, want the payee only", got)
	}
}

// The AI's values count like a rule's write: apply-rule replaces them without
// "include edited".
func TestAssignMovesAppliedSnapshot(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	other := h.category(t, userA, "Food")
	_, links := seedAssigned(t, h, c, cat, "")
	if tx := txOf(t, h, links[0].ID); !idIs(tx.CategoryID, cat) {
		t.Fatalf("assigned = %+v", tx)
	}
	l, _ := h.repo.GetLink(context.Background(), links[0].ID)
	if !idIs(l.AppliedCategoryID, cat) || l.AppliedPayeeID != nil || l.AppliedRuleID != nil {
		t.Fatalf("applied = category %v payee %v rule %v", l.AppliedCategoryID, l.AppliedPayeeID, l.AppliedRuleID)
	}
	uA := vo.MustParseId(userA)
	preview, err := h.svc.PreviewRule(context.Background(), uA, model.PreviewImportRuleRequest{ImportRuleSpec: model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee, MatchType: model.ImportRuleMatchTypeContains,
		MatchValue: "blue bottle", CategoryId: &other,
	}, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.Changes != 1 || preview.AlreadyEdited != 0 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: other})
	applied, err := h.svc.ApplyRule(context.Background(), uA, model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeAll})
	if err != nil || applied.Updated != 1 || applied.Skipped != 0 {
		t.Fatalf("apply = %+v, %v", applied, err)
	}
	if tx := txOf(t, h, links[0].ID); !idIs(tx.CategoryID, other) {
		t.Fatalf("apply kept the assigned category: %+v", tx)
	}
}

func TestAssignCreatesPayeeOnlyWhenWellKnown(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	coffee := h.category(t, userA, "Coffee")
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"LUNAS CORNER","merchants":["Lunas Corner Cafe"],"wellKnown":false,"categoryId":"` + coffee + `"}]}`
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "SQ *LUNAS CORNER CAFE"),
	)
	if len(h.payees.created) != 1 || h.payees.created[0] != "Whole Foods" {
		t.Fatalf("payees created = %v, want Whole Foods only", h.payees.created)
	}
	wf := txOf(t, h, bankLinkID(t, h, "a"))
	if !idIs(wf.CategoryID, groceries) || wf.PayeeID == nil {
		t.Fatalf("well-known row = %+v", wf)
	}
	if owners := payeeOwners(t, h, "Whole Foods"); len(owners) != 1 || owners[0] != userA {
		t.Fatalf("payee owners = %v", owners)
	}
	local := txOf(t, h, bankLinkID(t, h, "b"))
	if !idIs(local.CategoryID, coffee) || local.PayeeID != nil {
		t.Fatalf("local shop row = %+v, want a category and no payee", local)
	}
}

// A payee named like the merchant appeared while the AI was answering: the
// write re-reads the payees, every name compared normalized, and reuses it.
func TestAssignReusesPayeeByNormalisedName(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	var existing string
	c.onCall = func() { existing = h.payee(t, userA, "WHOLE FOODS!") }
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))
	if len(h.payees.created) != 0 {
		t.Fatalf("payees created = %v", h.payees.created)
	}
	if tx := txOf(t, h, bankLinkID(t, h, "a")); !idIs(tx.PayeeID, existing) {
		t.Fatalf("transaction = %+v, want the existing payee %s", tx, existing)
	}
}

func TestAssignArchivedPayeeMatchMeansNoPayee(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	h.archivedPayee(t, userA, "Whole Foods")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))
	if len(h.payees.created) != 0 {
		t.Fatalf("payees created = %v", h.payees.created)
	}
	if tx := txOf(t, h, bankLinkID(t, h, "a")); !idIs(tx.CategoryID, groceries) || tx.PayeeID != nil {
		t.Fatalf("transaction = %+v, want the category only", tx)
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 || got[0].PayeeID != nil || got[0].PayeeName != nil {
		t.Fatalf("rule suggestions = %+v, want no payee", got)
	}
}

func TestAssignOnePayeePerMerchantPerRun(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"WHOLEFDS","merchants":["WHOLE FOODS"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"2","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true}]}`
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "WHOLEFDS MKT 555"),
		extTx("ACT-1", "c", "-7.00", now.AddDate(0, 0, -3).Unix(), "WHOLEFDS #3"),
	)
	if len(h.payees.created) != 1 {
		t.Fatalf("payees created = %v, want one", h.payees.created)
	}
	first := txOf(t, h, bankLinkID(t, h, "a")).PayeeID
	for _, id := range []string{"a", "b", "c"} {
		if tx := txOf(t, h, bankLinkID(t, h, id)); first == nil || !idIs(tx.PayeeID, first.String()) {
			t.Fatalf("row %s = %+v, want payee %v", id, tx, first)
		}
	}
	// the rule suggestion carries the payee the run created, not its name
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || !idIs(got[0].PayeeID, first.String()) || got[0].PayeeName != nil || len(got[0].LinkIDs) != 3 {
		t.Fatalf("rule suggestions = %+v", got)
	}
}

func TestAssignOffWritesNothing(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	facts[userA] = ownerFact{language: "en", fillOff: true}
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))
	if tx := txOf(t, h, bankLinkID(t, h, "a")); tx.CategoryID != nil || tx.PayeeID != nil {
		t.Fatalf("transaction = %+v, want it untouched", tx)
	}
	if len(h.payees.created) != 0 || len(h.txns.updated) != 0 || len(assignRecords(t, h, userA)) != 0 {
		t.Fatalf("payees = %v, updates = %d, records = %+v", h.payees.created, len(h.txns.updated), assignRecords(t, h, userA))
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 || !idIs(got[0].CategoryID, groceries) {
		t.Fatalf("rule suggestions = %+v", got)
	}
}

func TestAssignSharedAccountGoesToOwner(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.accounts.shared = true
	if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB}); err != nil {
		t.Fatal(err)
	}
	h.category(t, userA, "Groceries")
	lebensmittel := h.category(t, userB, "Lebensmittel")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + lebensmittel + `"}]}`
	if res := ingest(t, h, pushWithPayee("WHOLEFDS MKT 10234", "evt-2")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	tx := txOf(t, h, links[0].ID)
	if !idIs(tx.CategoryID, lebensmittel) || tx.PayeeID == nil {
		t.Fatalf("transaction = %+v", tx)
	}
	if owners := payeeOwners(t, h, "Whole Foods"); len(owners) != 1 || owners[0] != userB {
		t.Fatalf("payee owners = %v, want the account owner", owners)
	}
	if len(h.txns.updatedBy) != 1 || h.txns.updatedBy[0].String() != userB {
		t.Fatalf("updated by = %v, want the account owner", h.txns.updatedBy)
	}
	if got := assignRecords(t, h, userB); len(got) != 1 || !idIs(got[0].CategoryID, lebensmittel) || !idIs(got[0].PayeeID, tx.PayeeID.String()) {
		t.Fatalf("owner records = %+v", got)
	}
	if got := assignRecords(t, h, userA); len(got) != 0 {
		t.Fatalf("importer records = %+v", got)
	}
}

// Edits made while the AI was answering win: a row set by hand in full is
// left alone (no payee created for it), a row with one field set gets the
// other.
func TestAssignSkipsTransactionEditedMeanwhile(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	coffee := h.category(t, userA, "Coffee")
	food := h.category(t, userA, "Food")
	mine := h.payee(t, userA, "My Market")
	c.onCall = func() {
		a := txOf(t, h, bankLinkID(t, h, "a"))
		h.setTx(t, &a.ID, "category_id", food)
		h.setTx(t, &a.ID, "payee_id", mine)
		b := txOf(t, h, bankLinkID(t, h, "b"))
		h.setTx(t, &b.ID, "category_id", food)
	}
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"STARBUCKS","merchants":["Starbucks"],"wellKnown":true,"categoryId":"` + coffee + `"}]}`
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "STARBUCKS 0042"),
	)
	if a := txOf(t, h, bankLinkID(t, h, "a")); !idIs(a.CategoryID, food) || !idIs(a.PayeeID, mine) {
		t.Fatalf("row edited in full = %+v", a)
	}
	b := txOf(t, h, bankLinkID(t, h, "b"))
	if !idIs(b.CategoryID, food) || b.PayeeID == nil {
		t.Fatalf("row with its category set = %+v, want the hand category and the AI payee", b)
	}
	if len(h.payees.created) != 1 || h.payees.created[0] != "Starbucks" {
		t.Fatalf("payees created = %v", h.payees.created)
	}
	got := assignRecords(t, h, userA)
	if len(got) != 1 || got[0].LinkID != bankLinkID(t, h, "b") || got[0].CategoryID != nil || !idIs(got[0].PayeeID, b.PayeeID.String()) {
		t.Fatalf("records = %+v", got)
	}
}

// A payee the payee feature refuses skips that row with a WARN that names no
// payee; the other rows are still assigned, and the rule suggestion keeps the
// name for the user to save.
func TestAssignPayeeFailureSkipsOnlyThatRow(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	coffee := h.category(t, userA, "Coffee")
	h.payees.fail = errors.New("refused Whole Foods")
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"CORNER SHOP","merchants":["Corner Shop"],"categoryId":"` + coffee + `"}]}`
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "CORNER SHOP"),
	)
	if a := txOf(t, h, bankLinkID(t, h, "a")); a.CategoryID != nil || a.PayeeID != nil {
		t.Fatalf("failed row = %+v, want it untouched", a)
	}
	if b := txOf(t, h, bankLinkID(t, h, "b")); !idIs(b.CategoryID, coffee) {
		t.Fatalf("other row = %+v", b)
	}
	out := buf.String()
	if !strings.Contains(out, "import assign row skipped") || !strings.Contains(out, "link_id="+bankLinkID(t, h, "a").String()) || strings.Contains(out, "Whole Foods") {
		t.Fatalf("log = %s", out)
	}
	var wf *model.ImportSuggestion
	for _, s := range openSuggestions(t, h, userA) {
		if strings.EqualFold(s.MatchText, "WHOLEFDS") {
			wf = &s
		}
	}
	if wf == nil || wf.PayeeID != nil || wf.PayeeName == nil || *wf.PayeeName != "Whole Foods" {
		t.Fatalf("rule suggestion = %+v", wf)
	}
}

// A row whose write fails after it created the merchant's payee rolls back
// to its own savepoint, payee included: the next row of that merchant creates
// it again, and only one is left.
func TestAssignFailedRowRollsBackItsPayee(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	h.txns.failUpdateAt = 1
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "WHOLEFDS MKT 555"),
	)
	if owners := payeeOwners(t, h, "Whole Foods"); len(owners) != 1 {
		t.Fatalf("payees named Whole Foods = %v, want one", owners)
	}
	if a := txOf(t, h, bankLinkID(t, h, "a")); a.CategoryID != nil || a.PayeeID != nil {
		t.Fatalf("failed row = %+v, want it untouched", a)
	}
	b := txOf(t, h, bankLinkID(t, h, "b"))
	if !idIs(b.CategoryID, groceries) || b.PayeeID == nil {
		t.Fatalf("second row = %+v", b)
	}
	if got := assignRecords(t, h, userA); len(got) != 1 || got[0].LinkID != bankLinkID(t, h, "b") {
		t.Fatalf("records = %+v, want only the second row", got)
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 || !idIs(got[0].PayeeID, b.PayeeID.String()) {
		t.Fatalf("rule suggestions = %+v", got)
	}
}

// Rows the reuse step joins to an open suggestion get its values without an
// AI call, only for the fields they lack.
func TestAssignFromOpenSuggestionOnReuse(t *testing.T) {
	h, c, _ := suggestSetup(t)
	h.mapCard(t, "Apple Card")
	cat := h.category(t, userA, "Coffee")
	ruled := h.payee(t, userA, "Blue Bottle Coffee")
	other := h.payee(t, userA, "Some Cafe")
	// the row arrives with its payee set, so only the category is missing
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", PayeeID: ruled})
	catID, otherID := vo.MustParseId(cat), vo.MustParseId(other)
	open := &model.ImportSuggestion{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE",
		CategoryID: &catID, PayeeID: &otherID, Status: model.ImportSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.repo.InsertSuggestion(context.Background(), open); err != nil {
		t.Fatal(err)
	}
	ingest(t, h, tap)
	if len(c.calls) != 0 {
		t.Fatalf("an open suggestion covers the row: %+v", c.calls)
	}
	links, _ := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if tx := txOf(t, h, links[0].ID); !idIs(tx.CategoryID, cat) || !idIs(tx.PayeeID, ruled) {
		t.Fatalf("transaction = %+v, want the suggestion's category and the rule's payee", tx)
	}
	got := assignRecords(t, h, userA)
	if len(got) != 1 || got[0].LinkID != links[0].ID || !idIs(got[0].CategoryID, cat) || got[0].PayeeID != nil || got[0].Status != model.ImportTransactionSuggestionStatusFilled {
		t.Fatalf("records = %+v, want the category only", got)
	}
}

// ClearSuggestions (AI turned off) deletes rule suggestions and open records
// left by earlier builds, never the record of what was assigned.
func TestClearSuggestionsKeepsAssignedRecords(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	_, links := seedAssigned(t, h, c, cat, "")
	other := h.f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: source, ExternalAccountID: "Apple Card", ExternalTransactionID: "leftover", Status: model.ImportLinkStatusSkipped, ExternalPostedAt: now})
	catID := vo.MustParseId(cat)
	if err := h.repo.InsertTransactionSuggestions(context.Background(), []model.ImportTransactionSuggestion{{
		LinkID: vo.MustParseId(other), UserID: vo.MustParseId(userA), CategoryID: &catID, Type: model.TransactionTypeExpense,
		Status: model.ImportTransactionSuggestionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ClearSuggestions(context.Background(), vo.MustParseId(userA)); err != nil {
		t.Fatal(err)
	}
	if got := openSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("rule suggestions = %+v", got)
	}
	got := assignRecords(t, h, userA)
	if len(got) != 1 || got[0].LinkID != links[0].ID || got[0].Status != model.ImportTransactionSuggestionStatusFilled {
		t.Fatalf("records = %+v, want only the filled one", got)
	}
}

func TestRuleSuggestionCarriesPayeeName(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	facts[userA] = ownerFact{language: "en", fillOff: true}
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))
	if len(h.payees.created) != 0 {
		t.Fatalf("payees created = %v, want none before the rule is saved", h.payees.created)
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].PayeeID != nil || got[0].PayeeName == nil || *got[0].PayeeName != "Whole Foods" || !idIs(got[0].CategoryID, groceries) {
		t.Fatalf("rule suggestions = %+v", got)
	}
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].PayeeName != "Whole Foods" || items[0].PayeeId != "" || len(items[0].Transactions) != 1 {
		t.Fatalf("list = %+v", items)
	}
}

// A well-known merchant with no fitting category and no payee still gets a
// rule suggestion: the new payee alone.
func TestRuleSuggestionPayeeNameAlone(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	facts[userA] = ownerFact{language: "en", fillOff: true}
	c.reply = `{"rows":[{"key":"0","matchText":"UBER","merchants":["Uber"],"wellKnown":true}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "UBER *TRIP 1234"))
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d: a payee-less row may get a new payee even with no payees", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].CategoryID != nil || got[0].PayeeName == nil || *got[0].PayeeName != "Uber" {
		t.Fatalf("rule suggestions = %+v", got)
	}
}

func TestRuleSuggestionPayeeNameResolvesToExistingPayee(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	facts[userA] = ownerFact{language: "en", fillOff: true}
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))

	byHand := h.payee(t, userA, "WHOLE FOODS")
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].PayeeId != byHand || items[0].PayeeName != "" {
		t.Fatalf("list = %+v, want the payee made by hand", items)
	}

	// with assignment on, a later row joins the suggestion and gets that payee
	facts[userA] = ownerFact{language: "en"}
	syncRows(t, h, p, extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 555"))
	if len(c.calls) != 1 {
		t.Fatalf("calls = %d, want the open suggestion reused", len(c.calls))
	}
	if tx := txOf(t, h, bankLinkID(t, h, "b")); !idIs(tx.PayeeID, byHand) || !idIs(tx.CategoryID, groceries) {
		t.Fatalf("joined row = %+v", tx)
	}
	if len(h.payees.created) != 0 {
		t.Fatalf("payees created = %v", h.payees.created)
	}
}

// With assignment on, a row joining an open suggestion whose new payee does
// not exist yet gets it created, once.
func TestAssignFromOpenSuggestionCreatesItsPayee(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	facts[userA] = ownerFact{language: "en", fillOff: true}
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))

	facts[userA] = ownerFact{language: "en"}
	syncRows(t, h, p,
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 555"),
		extTx("ACT-1", "c", "-7.00", now.AddDate(0, 0, -2).Unix(), "WHOLEFDS MKT 777"),
	)
	if len(c.calls) != 1 || len(h.payees.created) != 1 || h.payees.created[0] != "Whole Foods" {
		t.Fatalf("calls = %d, payees created = %v", len(c.calls), h.payees.created)
	}
	b, cc := txOf(t, h, bankLinkID(t, h, "b")), txOf(t, h, bankLinkID(t, h, "c"))
	if b.PayeeID == nil || !idIs(cc.PayeeID, b.PayeeID.String()) || !idIs(b.CategoryID, groceries) {
		t.Fatalf("joined rows = %+v, %+v", b, cc)
	}
	// the stored suggestion keeps its name and now reads as the new payee
	if items := suggestionList(t, h, userA); len(items) != 1 || items[0].PayeeId != b.PayeeID.String() || items[0].PayeeName != "" {
		t.Fatalf("list = %+v", items)
	}
}

func TestRuleSuggestionAgreesOnPayeeName(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	facts[userA] = ownerFact{language: "en", fillOff: true}
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p, extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"))
	link := bankLinkID(t, h, "a")
	tx := txOf(t, h, link)
	// the row carries the suggestion's values, its payee named like the
	// suggestion's new payee, and is on record as assigned
	wf := h.payee(t, userA, "whole foods")
	h.setTx(t, &tx.ID, "category_id", groceries)
	h.setTx(t, &tx.ID, "payee_id", wf)
	catID, wfID := vo.MustParseId(groceries), vo.MustParseId(wf)
	if err := h.repo.InsertTransactionSuggestions(context.Background(), []model.ImportTransactionSuggestion{{
		LinkID: link, UserID: vo.MustParseId(userA), CategoryID: &catID, PayeeID: &wfID, Type: model.TransactionTypeExpense,
		Status: model.ImportTransactionSuggestionStatusFilled, CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	if items := suggestionList(t, h, userA); len(items) != 1 || len(items[0].Transactions) != 1 {
		t.Fatalf("list = %+v, want the agreeing row kept", items)
	}

	other := h.payee(t, userA, "Corner Market")
	h.setTx(t, &tx.ID, "payee_id", other)
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("list = %+v, want the disagreeing row gone", items)
	}
}

func TestRuleSuggestionsStayListedForAssignedRows(t *testing.T) {
	h, p, c, _ := assignSetup(t)
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"WHOLEFDS","merchants":["Whole Foods"],"wellKnown":true,"categoryId":"` + groceries + `"}]}`
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "WHOLEFDS MKT 555"),
	)
	a := txOf(t, h, bankLinkID(t, h, "a"))
	if a.PayeeID == nil || !idIs(a.CategoryID, groceries) {
		t.Fatalf("assigned = %+v", a)
	}
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].PayeeId != a.PayeeID.String() || items[0].PayeeName != "" || items[0].CategoryId != groceries || len(items[0].Transactions) != 2 {
		t.Fatalf("list = %+v", items)
	}
}

// lockOrderRepo records the transaction row locks and the link writes, in order.
type lockOrderRepo struct {
	imports.Repository
	events []string
}

func (r *lockOrderRepo) LockTransactionRow(ctx context.Context, id vo.Id) error {
	r.events = append(r.events, "lock "+id.String())
	return r.Repository.LockTransactionRow(ctx, id)
}

func (r *lockOrderRepo) UpdateLink(ctx context.Context, l *model.ImportTransactionLink) error {
	r.events = append(r.events, "write")
	return r.Repository.UpdateLink(ctx, l)
}

// Every row of a run is locked, in id order, before the first write: a write
// bumps the account, and waiting for a later row's lock while holding the
// account could deadlock with a hand edit holding that row (PostgreSQL).
func TestAssignLocksAllRowsInIdOrderBeforeWriting(t *testing.T) {
	h, p, c, facts := assignSetup(t)
	rec := &lockOrderRepo{Repository: h.repo}
	h.withRepo(rec)
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	h.wireSuggest(c, facts)
	groceries := h.category(t, userA, "Groceries")
	c.reply = `{"rows":[` +
		`{"key":"0","matchText":"WHOLEFDS","categoryId":"` + groceries + `"},` +
		`{"key":"1","matchText":"WHOLEFDS","categoryId":"` + groceries + `"},` +
		`{"key":"2","matchText":"WHOLEFDS","categoryId":"` + groceries + `"}]}`
	// the job takes the newest row first: the reverse of the order the rows were created in
	syncRows(t, h, p,
		extTx("ACT-1", "a", "-5.00", now.AddDate(0, 0, -3).Unix(), "WHOLEFDS MKT 10234"),
		extTx("ACT-1", "b", "-6.00", now.AddDate(0, 0, -2).Unix(), "WHOLEFDS MKT 555"),
		extTx("ACT-1", "c", "-7.00", now.AddDate(0, 0, -1).Unix(), "WHOLEFDS #3"),
	)
	var want []string
	for _, id := range []string{"a", "b", "c"} {
		tx := txOf(t, h, bankLinkID(t, h, id))
		if !idIs(tx.CategoryID, groceries) {
			t.Fatalf("row %s = %+v, want it filled", id, tx)
		}
		want = append(want, "lock "+tx.ID.String())
	}
	sort.Strings(want)
	first := slices.Index(rec.events, "write")
	// each row locks again on its own (a no-op by then)
	if first < len(want) || !slices.Equal(rec.events[:len(want)], want) {
		t.Fatalf("events = %v, want the locks %v before the first write", rec.events, want)
	}
}
