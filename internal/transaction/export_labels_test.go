package transaction

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// TestExportJoinsLabelNamesWithSemicolon locks the labels cell's join order:
// it follows the owner's LIST ORDER, not the id order LabelsByTransactionIDs returns
// (ascending label_id, a determinism guarantee, not a display order) and not
// name order either. The two label ids below are listed in ASCENDING id order
// (matching what the repo would hand back) but their sort keys are reversed
// relative to that, so a cell that merely echoed the input order — or sorted
// by id or name — would emit "Kid B;Kid A" instead of the expected
// "Kid A;Kid B".
func TestExportJoinsLabelNamesWithSemicolon(t *testing.T) {
	const (
		txID     = "aaaa0000-0000-0000-0000-000000000001"
		idLower  = "bbbb0000-0000-0000-0000-000000000001" // sorts first by id
		idHigher = "cccc0000-0000-0000-0000-000000000002" // sorts second by id
	)
	idx := exportLabelIndex{
		byTx: map[string][]string{txID: {idLower, idHigher}},
		byID: map[string]model.ExportLabel{
			idLower:  {Name: "Kid B", SortKey: "a1"},
			idHigher: {Name: "Kid A", SortKey: "a0"},
		},
	}
	if got := idx.cell(txID); got != "Kid A;Kid B" {
		t.Fatalf("cell = %q, want %q (list order, not id order)", got, "Kid A;Kid B")
	}
}

// TestExportSanitizesEachLabelNameBeforeJoining locks the formula-injection
// guard's placement: each label name is sanitized BEFORE the ";" join, not
// after. A name starting with "=" only defuses if the guard runs per-name —
// sanitizing the already-joined string only inspects byte 0 of the whole
// cell, so a leading "=" on any name after the first would slip through
// unescaped. "Safe" (first) does not trigger the guard on its own, so
// this case can only pass if "=cmd" (second) is individually defused.
func TestExportSanitizesEachLabelNameBeforeJoining(t *testing.T) {
	const (
		txID    = "aaaa0000-0000-0000-0000-000000000002"
		safeID  = "bbbb0000-0000-0000-0000-000000000003"
		evilID  = "cccc0000-0000-0000-0000-000000000004"
		wantCol = "Safe;'=cmd"
	)
	idx := exportLabelIndex{
		byTx: map[string][]string{txID: {safeID, evilID}},
		byID: map[string]model.ExportLabel{
			safeID: {Name: "Safe", SortKey: "a0"},
			evilID: {Name: "=cmd", SortKey: "a1"},
		},
	}
	if got := idx.cell(txID); got != wantCol {
		t.Fatalf("cell = %q, want %q (per-name sanitize before join)", got, wantCol)
	}
}

// TestExportLabelIndexCell_NoLabels covers the empty case: a transaction id
// absent from byTx (no labels attached) yields "".
func TestExportLabelIndexCell_NoLabels(t *testing.T) {
	idx := exportLabelIndex{byTx: map[string][]string{}, byID: map[string]model.ExportLabel{}}
	if got := idx.cell("no-such-tx"); got != "" {
		t.Fatalf("cell = %q, want empty", got)
	}
}

// TestExportWritesEmptyLabelsForTransferRecipientRow: a transfer's source row
// carries its resolved labels cell, but the recipient row -- built from the
// same transaction, same label index -- always writes "" for labels, exactly
// as it already does for tag/category/payee. Both accounts are selected so
// buildExportRows emits both rows.
func TestExportWritesEmptyLabelsForTransferRecipientRow(t *testing.T) {
	txID := vo.NewId()
	srcAccountID := vo.NewId()
	recipAccountID := vo.NewId()
	const labelID = "bbbb0000-0000-0000-0000-000000000005"

	tx := &model.Transaction{
		ID:             txID,
		Type:           model.TransactionTypeTransfer,
		AccountID:      srcAccountID,
		AccountRecipID: &recipAccountID,
		Amount:         "10.00",
	}

	src := model.ExportAccount{ID: srcAccountID.String(), Name: "Checking", CurrencyCode: "USD"}
	recip := model.ExportAccount{ID: recipAccountID.String(), Name: "Savings", CurrencyCode: "USD"}
	selectedByID := map[string]model.ExportAccount{src.ID: src, recip.ID: recip}
	allAccountsByID := map[string]model.ExportAccount{src.ID: src, recip.ID: recip}

	labels := exportLabelIndex{
		byTx: map[string][]string{txID.String(): {labelID}},
		byID: map[string]model.ExportLabel{labelID: {Name: "Kid A", SortKey: "a0"}},
	}

	svc := &Service{}
	rows, err := svc.buildExportRows(context.Background(), tx, selectedByID, allAccountsByID, newExportNameCache(), labels)
	if err != nil {
		t.Fatalf("buildExportRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (source + recipient)", len(rows))
	}
	// Column 6 is "labels" (transaction_id, account_name, account_currency,
	// category, description, tag, labels, payee, amount, date).
	const labelsCol = 6
	sourceRow, recipRow := rows[0], rows[1]
	if sourceRow[1] != "Checking" {
		t.Fatalf("rows[0] account_name = %q, want the source row first", sourceRow[1])
	}
	if sourceRow[labelsCol] != "Kid A" {
		t.Fatalf("source row labels = %q, want %q", sourceRow[labelsCol], "Kid A")
	}
	if recipRow[1] != "Savings" {
		t.Fatalf("rows[1] account_name = %q, want the recipient row second", recipRow[1])
	}
	if recipRow[labelsCol] != "" {
		t.Fatalf("recipient row labels = %q, want empty", recipRow[labelsCol])
	}
}
