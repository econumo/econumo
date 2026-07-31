package model

import (
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

// Positions are persisted into an int16 column; a value outside its range must be
// rejected at validation rather than wrapping silently on conversion.
func TestOrderRequests_RejectOutOfRangePosition(t *testing.T) {
	const overflow = 32768   // int16 max + 1
	const underflow = -32769 // int16 min - 1
	const ok = 100

	assertRejected := func(t *testing.T, err error) {
		t.Helper()
		v, isVal := errs.AsValidation(err)
		if !isVal {
			t.Fatalf("err = %v, want *errs.ValidationError", err)
		}
		for _, f := range v.Fields {
			if f.Key == "position" && f.Code == errs.CodeOutOfRange {
				return
			}
		}
		t.Fatalf("no position out-of-range field error; got %+v", v.Fields)
	}

	t.Run("category overflow", func(t *testing.T) {
		assertRejected(t, OrderCategoryListRequest{Changes: []PositionChange{{Id: "x", Position: overflow}}}.Validate())
	})
	t.Run("tag underflow", func(t *testing.T) {
		assertRejected(t, OrderTagListRequest{Changes: []PositionChange{{Id: "x", Position: underflow}}}.Validate())
	})
	t.Run("payee overflow", func(t *testing.T) {
		assertRejected(t, OrderPayeeListRequest{Changes: []PositionChange{{Id: "x", Position: overflow}}}.Validate())
	})
	t.Run("account overflow", func(t *testing.T) {
		assertRejected(t, OrderAccountListRequest{Changes: []AccountPositionChange{{Id: "x", FolderId: "f", Position: overflow}}}.Validate())
	})
	t.Run("budget move overflow", func(t *testing.T) {
		assertRejected(t, MoveElementListRequest{BudgetId: "b", Items: []MoveElementListItem{{Id: "x", Position: overflow}}}.Validate())
	})

	t.Run("in-range accepted", func(t *testing.T) {
		if err := (OrderCategoryListRequest{Changes: []PositionChange{{Id: "x", Position: ok}}}).Validate(); err != nil {
			t.Fatalf("in-range position rejected: %v", err)
		}
	})
}
