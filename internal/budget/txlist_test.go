package budget

// Covers the selector-combination guard at the top of GetTransactionList. The
// guard runs before the budget is loaded, so a zero Service suffices: a
// combination that reaches the budget lookup has been ACCEPTED by the guard,
// which is exactly what these tests distinguish.

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	txListLabelID    = "eeee7777-0000-7000-8000-000000000001"
	txListCategoryID = "cccc7777-0000-7000-8000-000000000001"
	txListTagID      = "dddd7777-0000-7000-8000-000000000001"
	txListEnvelopeID = "bbbb7777-0000-7000-8000-000000000001"
	txListBudgetID   = "aaaa7777-0000-7000-8000-000000000001"
)

func strp(s string) *string { return &s }

// filterRejected reports whether GetTransactionList refused the combination up
// front with CodeBudgetTransactionFilterRequired. Any other outcome (including
// the panic/error from the un-stubbed repository the accepted path reaches)
// means the guard let the request through.
func filterRejected(t *testing.T, req model.BudgetTransactionListRequest) bool {
	t.Helper()
	s := &Service{}
	var rejected bool
	func() {
		defer func() { _ = recover() }()
		_, err := s.GetTransactionList(context.Background(), vo.MustParseId(txListUserID), req)
		var ve *errs.ValidationError
		if errors.As(err, &ve) && ve.MsgCode == errs.CodeBudgetTransactionFilterRequired {
			rejected = true
		}
	}()
	return rejected
}

const txListUserID = "11117777-0000-7000-8000-000000000001"

func baseTxListRequest() model.BudgetTransactionListRequest {
	return model.BudgetTransactionListRequest{BudgetId: txListBudgetID, PeriodStart: "2024-04-01"}
}

// A reporting tag narrows by category (that is the folder drill-down), so
// these two pairings must pass the guard.
func TestTxListGuardAllowsLabelWithCategoryAndUncategorized(t *testing.T) {
	withCat := baseTxListRequest()
	withCat.LabelId = strp(txListLabelID)
	withCat.CategoryId = strp(txListCategoryID)
	if filterRejected(t, withCat) {
		t.Fatalf("labelId+categoryId was rejected; it must be allowed (the folder's category row drills down to that pair)")
	}

	withUncat := baseTxListRequest()
	withUncat.LabelId = strp(txListLabelID)
	withUncat.Uncategorized = true
	if filterRejected(t, withUncat) {
		t.Fatalf("labelId+uncategorized was rejected; it must be allowed (the folder's uncategorized row)")
	}
}

// Pairing a reporting tag with another SELECTOR stays ambiguous about which
// one narrows, so both combinations must keep failing.
func TestTxListGuardStillRejectsLabelWithTagAndEnvelope(t *testing.T) {
	withTag := baseTxListRequest()
	withTag.LabelId = strp(txListLabelID)
	withTag.TagId = strp(txListTagID)
	if !filterRejected(t, withTag) {
		t.Fatalf("labelId+tagId must stay rejected with CodeBudgetTransactionFilterRequired")
	}

	withEnv := baseTxListRequest()
	withEnv.LabelId = strp(txListLabelID)
	withEnv.EnvelopeId = strp(txListEnvelopeID)
	if !filterRejected(t, withEnv) {
		t.Fatalf("labelId+envelopeId must stay rejected with CodeBudgetTransactionFilterRequired")
	}
}

// The pre-existing uncategorized+categoryId guard is a different error (a
// field-specific one) and must survive the relaxation untouched, with or
// without a labelId alongside.
func TestTxListGuardKeepsUncategorizedWithCategoryRejected(t *testing.T) {
	req := baseTxListRequest()
	req.Uncategorized = true
	req.CategoryId = strp(txListCategoryID)
	s := &Service{}
	_, err := s.GetTransactionList(context.Background(), vo.MustParseId(txListUserID), req)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("uncategorized+categoryId error = %v, want a validation error", err)
	}
	if len(ve.Fields) == 0 || ve.Fields[0].Key != "categoryId" {
		t.Fatalf("uncategorized+categoryId must keep its categoryId field error, got %+v", ve.Fields)
	}
}

// A whitespace-only labelId must read as absent to the guard AND to the
// selector switch, so it can neither trip the new label cases nor be silently
// swallowed by an earlier one.
func TestTxListGuardTreatsWhitespaceLabelAsAbsent(t *testing.T) {
	req := baseTxListRequest()
	req.LabelId = strp("   ")
	req.TagId = strp(txListTagID)
	if filterRejected(t, req) {
		t.Fatalf("whitespace-only labelId must not count as a label selector: tagId alone is a valid filter")
	}
}
