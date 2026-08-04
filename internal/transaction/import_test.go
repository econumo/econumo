package transaction

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// newLabelCapTestService builds a fresh Service + stub importer per subtest
// (not shared) so createLabelCalls from one case can't leak into another's
// assertions. The account/date overrides sidestep account-name resolution
// (CreateAccount) so the test isolates the labels cap.
func newLabelCapTestService() (svc *Service, imp *stubImportLabelOwner, userID vo.Id, acctIDStr, dateStr string) {
	userID = vo.NewId()
	acctID := vo.NewId()
	imp = &stubImportLabelOwner{
		account:       &model.ImportAccount{ID: acctID.String(), Name: "Checking", OwnerID: userID.String()},
		labelsByOwner: map[string][]model.ImportNamed{},
		nextLabelID:   vo.NewId(),
	}
	svc = &Service{importer: imp, repo: &stubImportRepo{}, tx: passthroughTx{}, clock: fixedClock{now: time.Now()}}
	return svc, imp, userID, acctID.String(), "2024-03-01"
}

// labelsCell joins n distinct piece names, e.g. labelsCell(3) -> "L1;L2;L3".
func labelsCell(n int) string {
	pieces := make([]string, n)
	for i := 0; i < n; i++ {
		pieces[i] = fmt.Sprintf("L%d", i+1)
	}
	return strings.Join(pieces, ";")
}

// TestImportRejectsRowWithTooManyLabels pins maxLabelsPerImportRow: a row
// whose mapped labels cell resolves to more than 10 DISTINCT pieces fails as
// a row-level error (not a silent truncation, and not a whole-import abort),
// while the count is taken on the deduped list splitLabelCell already
// produces, not the raw split - dedup collapsing raw pieces below the cap
// must not trip it.
func TestImportRejectsRowWithTooManyLabels(t *testing.T) {
	t.Run("11 distinct pieces -> that row fails, the rest of the import still proceeds", func(t *testing.T) {
		svc, imp, userID, acctIDStr, dateStr := newLabelCapTestService()
		csv := "Amount,Labels\n-10," + labelsCell(11) + "\n-20,Solo\n"
		req := model.ImportRequest{
			File:      []byte(csv),
			Mapping:   model.ImportMapping{Amount: "Amount", Labels: "Labels"},
			AccountId: &acctIDStr,
			Date:      &dateStr,
		}

		res, err := svc.ImportTransactionList(context.Background(), userID, req)
		if err != nil {
			t.Fatalf("ImportTransactionList: %v", err)
		}
		if res.Imported != 1 || res.Skipped != 1 {
			t.Fatalf("imported=%d skipped=%d, want 1/1 (row 1 rejected, row 2 still imported); errors=%v", res.Imported, res.Skipped, res.Errors)
		}
		var rows []int
		for msg, r := range res.Errors {
			if strings.Contains(msg, "maximum of 10") {
				rows = r
			}
		}
		if len(rows) != 1 || rows[0] != 2 {
			t.Fatalf("cap error rows = %v, want [2] (the first data row, 1-indexed header + 1); errors=%v", rows, res.Errors)
		}
		// Only "Solo" (row 2) should have created a label; none of the 11
		// pieces from the rejected row 1 must have been partially created.
		if len(imp.createLabelCalls) != 1 || imp.createLabelCalls[0].name != "Solo" {
			t.Fatalf("createLabelCalls = %v, want exactly one call for \"Solo\" (row 1's pieces must not be partially created)", imp.createLabelCalls)
		}
	})

	t.Run("exactly 10 distinct pieces -> accepted", func(t *testing.T) {
		svc, imp, userID, acctIDStr, dateStr := newLabelCapTestService()
		csv := "Amount,Labels\n-10," + labelsCell(10) + "\n"
		req := model.ImportRequest{
			File:      []byte(csv),
			Mapping:   model.ImportMapping{Amount: "Amount", Labels: "Labels"},
			AccountId: &acctIDStr,
			Date:      &dateStr,
		}

		res, err := svc.ImportTransactionList(context.Background(), userID, req)
		if err != nil {
			t.Fatalf("ImportTransactionList: %v", err)
		}
		if res.Imported != 1 || res.Skipped != 0 || len(res.Errors) != 0 {
			t.Fatalf("imported=%d skipped=%d errors=%v, want 1/0/{} (10 is within the cap)", res.Imported, res.Skipped, res.Errors)
		}
		if len(imp.createLabelCalls) != 10 {
			t.Fatalf("createLabelCalls = %d, want 10", len(imp.createLabelCalls))
		}
	})

	t.Run(`"A;a" dedupes to 1 -> accepted (the cap counts deduped values, not raw pieces)`, func(t *testing.T) {
		svc, imp, userID, acctIDStr, dateStr := newLabelCapTestService()
		csv := "Amount,Labels\n-10,A;a\n"
		req := model.ImportRequest{
			File:      []byte(csv),
			Mapping:   model.ImportMapping{Amount: "Amount", Labels: "Labels"},
			AccountId: &acctIDStr,
			Date:      &dateStr,
		}

		res, err := svc.ImportTransactionList(context.Background(), userID, req)
		if err != nil {
			t.Fatalf("ImportTransactionList: %v", err)
		}
		if res.Imported != 1 || res.Skipped != 0 || len(res.Errors) != 0 {
			t.Fatalf("imported=%d skipped=%d errors=%v, want 1/0/{}", res.Imported, res.Skipped, res.Errors)
		}
		if len(imp.createLabelCalls) != 1 {
			t.Fatalf("createLabelCalls = %d, want 1 (\"A\" and \"a\" are the same label)", len(imp.createLabelCalls))
		}
	})

	t.Run("11 raw pieces that dedupe to 1 distinct label -> accepted", func(t *testing.T) {
		// A stronger version of the "A;a" case: 11 raw split pieces (over the
		// cap) that all case-fold to the same label. If the cap ever counted
		// the raw split instead of splitLabelCell's deduped output, this row
		// would wrongly fail.
		svc, imp, userID, acctIDStr, dateStr := newLabelCapTestService()
		pieces := make([]string, 11)
		for i := range pieces {
			if i%2 == 0 {
				pieces[i] = "A"
			} else {
				pieces[i] = "a"
			}
		}
		csv := "Amount,Labels\n-10," + strings.Join(pieces, ";") + "\n"
		req := model.ImportRequest{
			File:      []byte(csv),
			Mapping:   model.ImportMapping{Amount: "Amount", Labels: "Labels"},
			AccountId: &acctIDStr,
			Date:      &dateStr,
		}

		res, err := svc.ImportTransactionList(context.Background(), userID, req)
		if err != nil {
			t.Fatalf("ImportTransactionList: %v", err)
		}
		if res.Imported != 1 || res.Skipped != 0 || len(res.Errors) != 0 {
			t.Fatalf("imported=%d skipped=%d errors=%v, want 1/0/{} (11 raw pieces dedupe to 1)", res.Imported, res.Skipped, res.Errors)
		}
		if len(imp.createLabelCalls) != 1 {
			t.Fatalf("createLabelCalls = %d, want 1", len(imp.createLabelCalls))
		}
	})
}

// TestImportReportsInvalidLabelNameAsRowError pins that a label-name
// validation failure from CreateLabel (internal/label's 3-64 rune rule)
// propagates as a row-level error rather than being swallowed - the row is
// skipped and recorded, but the rest of the import still proceeds.
func TestImportReportsInvalidLabelNameAsRowError(t *testing.T) {
	userID := vo.NewId()
	acctID := vo.NewId()
	imp := &stubImportLabelOwner{
		account:       &model.ImportAccount{ID: acctID.String(), Name: "Checking", OwnerID: userID.String()},
		labelsByOwner: map[string][]model.ImportNamed{},
		nextLabelID:   vo.NewId(),
		createLabelErrFor: map[string]error{
			"AB": errs.NewValidation("Label name must be 3-64 characters",
				errs.FieldError{
					Key: "name", Message: "Label name must be 3-64 characters", Code: errs.CodeLabelNameLength,
					Params: map[string]any{"min": 3, "max": 64},
				}),
		},
	}
	svc := &Service{importer: imp, repo: &stubImportRepo{}, tx: passthroughTx{}, clock: fixedClock{now: time.Now()}}

	acctIDStr := acctID.String()
	dateStr := "2024-03-01"
	csv := "Amount,Labels\n-10,AB\n-20,Valid Label\n"
	req := model.ImportRequest{
		File:      []byte(csv),
		Mapping:   model.ImportMapping{Amount: "Amount", Labels: "Labels"},
		AccountId: &acctIDStr,
		Date:      &dateStr,
	}

	res, err := svc.ImportTransactionList(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("ImportTransactionList: %v", err)
	}
	if res.Imported != 1 || res.Skipped != 1 {
		t.Fatalf("imported=%d skipped=%d, want 1/1 (row 1 rejected, row 2 still imported); errors=%v", res.Imported, res.Skipped, res.Errors)
	}
	var rows []int
	for msg, r := range res.Errors {
		if strings.Contains(msg, "3-64 characters") {
			rows = r
		}
	}
	if len(rows) != 1 || rows[0] != 2 {
		t.Fatalf("name-length error rows = %v, want [2]; errors=%v", rows, res.Errors)
	}
}
