package imports_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// A bank sync that adopts the AI-filled transaction and corrects its amount
// writes a ledger row of its own; the tap's row stays the AI's, and the rule
// suggestion still offers the transaction.
func TestAssignedByKeptWhenABankSyncAdoptsTheRow(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedAssigned(t, h, c, cat, "")
	tapLink := links[0].ID
	if got := assignedBy(t, h, tapLink); got != model.ImportAssignedByAI {
		t.Fatalf("assigned by = %q, want ai", got)
	}

	h.f.ImportSource(fixture.ImportSource{ID: bankSource, UserID: userA, Provider: model.ImportProviderSimpleFIN, Name: "Bank", CredentialCiphertext: "v1:iv:ct"})
	p := &fakeProvider{accounts: []model.ExternalAccount{{ID: "ACT-1", Name: "Checking", Currency: "USD", Balance: "10", OrgName: "Big Bank"}}}
	h.svc.RegisterProvider(model.ImportProviderSimpleFIN, p)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: bankSource, ExternalAccountID: "ACT-1", AccountID: acct1})
	h.txns.candidates = []*model.Transaction{txOf(t, h, tapLink)}
	p.txs = []model.ExternalTransaction{extTx("ACT-1", "T1", "-5.25", time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC).Unix(), "Blue Bottle")}
	res, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq())
	if err != nil || res.Run.MatchedCount != 1 || res.Run.AmountsUpdatedCount != 1 {
		t.Fatalf("sync = %+v, %v", res, err)
	}
	// and a second fetch of the same bank row is a duplicate
	p.txs[0].Amount = "-5.30"
	if _, err := h.svc.Sync(context.Background(), vo.MustParseId(userA), syncReq()); err != nil {
		t.Fatal(err)
	}

	if got := assignedBy(t, h, tapLink); got != model.ImportAssignedByAI {
		t.Fatalf("tap row assigned by = %q after the sync, want ai", got)
	}
	if got := assignedBy(t, h, bankLinkID(t, h, "T1")); got != model.ImportAssignedByNone {
		t.Fatalf("adopting row assigned by = %q, want none", got)
	}
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].Id != s.ID.String() || len(items[0].Transactions) != 1 || items[0].Transactions[0].TransactionId != links[0].TransactionID.String() {
		t.Fatalf("list = %+v, want the assigned row still offered", items)
	}
}

// Apply-rule writing a field the AI left empty takes the row over.
func TestAssignedByApplyRuleAfterTheAI(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	_, links := seedAssigned(t, h, c, cat, "")
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", PayeeID: py})
	res, err := h.svc.ApplyRule(context.Background(), vo.MustParseId(userA), model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeAll})
	if err != nil || res.Updated != 1 {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	if got := assignedBy(t, h, links[0].ID); got != model.ImportAssignedByRule {
		t.Fatalf("assigned by = %q, want rule", got)
	}
}

// Re-stamping a hand-edited row changes nothing on the transaction, but the
// snapshot now names the rule, and so does assigned_by.
func TestAssignedByRuleWhenApplyRuleRestamps(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	_, links := seedAssigned(t, h, c, cat, "")
	h.setTx(t, links[0].TransactionID, "payee_id", py)
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", PayeeID: py})
	res, err := h.svc.ApplyRule(context.Background(), vo.MustParseId(userA), model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeAll, IncludeEdited: true})
	if err != nil || res.Updated != 0 {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	l, _ := h.repo.GetLink(context.Background(), links[0].ID)
	if l.AppliedRuleID == nil || l.AppliedRuleID.String() != rule || !idIs(l.AppliedPayeeID, py) {
		t.Fatalf("applied = payee %v rule %v, want the rule's baseline", l.AppliedPayeeID, l.AppliedRuleID)
	}
	if l.AssignedBy != model.ImportAssignedByRule {
		t.Fatalf("assigned by = %q, want rule", l.AssignedBy)
	}
}

// Accepting a rule suggestion that fills a field the AI left empty takes the
// row over; one with nothing to fill leaves it the AI's.
func TestAssignedByAcceptAfterTheAI(t *testing.T) {
	for _, tc := range []struct {
		name        string
		withPayee   bool
		wantUpdated int
		want        model.ImportAssignedBy
	}{{"fills", true, 1, model.ImportAssignedByRule}, {"nothing to fill", false, 0, model.ImportAssignedByAI}} {
		t.Run(tc.name, func(t *testing.T) {
			h, c, _ := suggestSetup(t)
			cat := h.category(t, userA, "Coffee")
			s, links := seedAssigned(t, h, c, cat, "")
			r := fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat}
			if tc.withPayee {
				r.PayeeID = h.payee(t, userA, "Blue Bottle Coffee")
			}
			rule := h.f.ImportRule(r)
			res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
			if err != nil || res.Updated != tc.wantUpdated {
				t.Fatalf("accept = %+v, %v", res, err)
			}
			if got := assignedBy(t, h, links[0].ID); got != tc.want {
				t.Fatalf("assigned by = %q, want %q", got, tc.want)
			}
		})
	}
}

// A skip rule's id sits on the row, but nothing was classified.
func TestAssignedBySkipRowStaysNone(t *testing.T) {
	h := setup(t)
	h.mapCard(t, "Apple Card")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: model.ImportRuleActionSkip, MatchValue: "blue bottle"})
	if res := ingest(t, h, tap); res.Status != model.ImportIngestStatusSkipped {
		t.Fatalf("ingest = %+v", res)
	}
	l := linkByExternalID(t, h, "evt-1")
	if l.AppliedRuleID == nil || l.AssignedBy != model.ImportAssignedByNone {
		t.Fatalf("skip row: rule %v assigned by %q, want the rule id and none", l.AppliedRuleID, l.AssignedBy)
	}
}

// A queued tap the mapping places: a rule that classifies it takes it, an
// unruled one stays none, and a skip stays none.
func TestAssignedByConvertQueued(t *testing.T) {
	h := setup(t)
	cat := h.category(t, userA, "Coffee")
	h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat})
	h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: model.ImportRuleActionSkip, MatchValue: "skip me"})
	ingest(t, h, tap)
	ingest(t, h, pushWithPayee("Corner Shop", "evt-2"))
	ingest(t, h, pushWithPayee("Skip Me", "evt-3"))
	if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acct1}); err != nil {
		t.Fatal(err)
	}
	for ext, want := range map[string]struct {
		status string
		ruled  bool
		by     model.ImportAssignedBy
	}{
		"evt-1": {model.ImportLinkStatusLinked, true, model.ImportAssignedByRule},
		"evt-2": {model.ImportLinkStatusLinked, false, model.ImportAssignedByNone},
		"evt-3": {model.ImportLinkStatusSkipped, true, model.ImportAssignedByNone},
	} {
		l := linkByExternalID(t, h, ext)
		if l.Status != want.status || (l.AppliedRuleID != nil) != want.ruled || l.AssignedBy != want.by {
			t.Fatalf("%s: status %s rule %v assigned by %q, want %+v", ext, l.Status, l.AppliedRuleID, l.AssignedBy, want)
		}
	}
}

// Reimport rebuilds the row under the same id from the import's own data: an
// earlier job's value does not carry over.
func TestAssignedByReimportStartsFresh(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	_, links := seedAssigned(t, h, c, cat, "")
	if got := assignedBy(t, h, links[0].ID); got != model.ImportAssignedByAI {
		t.Fatalf("assigned by = %q, want ai", got)
	}
	ctx := context.Background()
	if _, err := h.db.Raw.ExecContext(ctx, h.db.Rebind(`DELETE FROM transactions WHERE id = ?`), links[0].TransactionID.String()); err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.ReimportLink(ctx, vo.MustParseId(userA), model.ImportLinkActionRequest{LinkId: links[0].ID.String()})
	if err != nil || res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("reimport = %+v, %v", res, err)
	}
	if got := assignedBy(t, h, links[0].ID); got != model.ImportAssignedByNone {
		t.Fatalf("reimported row assigned by = %q, want none", got)
	}
}
