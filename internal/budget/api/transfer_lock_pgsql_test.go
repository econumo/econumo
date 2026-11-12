//go:build enginecompare

package api_test

import (
	"context"
	"testing"
	"time"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// A plan transfer racing a limit write on one of its rows: the writer has
// saved its plan and is still open when the transfer starts. The transfer must
// wait for it and move what it wrote. Without the row locks it read the old
// plans first and wrote them back over the new one.
func TestTransferElementData_WaitsForAConcurrentLimitWrite(t *testing.T) {
	for _, c := range []struct {
		name        string
		writeOnDest bool
		want        string // the target's plan after both commit
	}{
		{"write on the source", false, "95"},     // 20 + the new 75
		{"write on the destination", true, "85"}, // the new 75 + 10
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newSavingsLockRig(t)
			ctx := context.Background()
			f := fixture.New(t, r.db1)
			src := f.Category(fixture.Category{UserID: seedUserID, Name: "Loan"})
			dst := f.Category(fixture.Category{UserID: seedUserID, Name: "Debt"})
			setLimit := func(svc *appbudget.Service, element, amount string) error {
				a := vo.NewFlexString(amount)
				_, err := svc.SetLimit(ctx, r.userID, model.SetLimitRequest{BudgetId: budgetID1, ElementId: element, Period: "2026-08-01", Amount: &a})
				return err
			}
			if err := setLimit(r.svc1, src, "10"); err != nil {
				t.Fatal(err)
			}
			if err := setLimit(r.svc1, dst, "20"); err != nil {
				t.Fatal(err)
			}

			saved := make(chan struct{})
			release, unblock := gate(t)
			r.repo2.afterSaveLimit = func() {
				close(saved)
				<-release
			}
			target := src
			if c.writeOnDest {
				target = dst
			}
			wrote := make(chan error, 1)
			go func() { wrote <- setLimit(r.svc2, target, "75") }()
			select {
			case <-saved:
			case err := <-wrote:
				t.Fatalf("set-limit finished before its save hook: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("set-limit never saved")
			}

			moved := make(chan error, 1)
			go func() {
				_, err := r.svc1.TransferElementData(ctx, model.ElementDataTransfer{
					BudgetID: r.budgetID, FromID: vo.MustParseId(src), FromType: model.ElementCategory, ToID: vo.MustParseId(dst), ToType: model.ElementCategory,
					CurrencyID: vo.MustParseId(usdID), SumOnClash: true,
				})
				moved <- err
			}()
			r.waitLockWait(t, r.pid1, "transfer", moved)

			unblock()
			if err := <-wrote; err != nil {
				t.Fatalf("set-limit: %v", err)
			}
			if err := <-moved; err != nil {
				t.Fatalf("transfer: %v", err)
			}
			want := map[string]string{src: "", dst: c.want}
			for id, w := range want {
				var got string
				err := r.monitor.QueryRow(`SELECT COALESCE((SELECT l.amount::text FROM budgets_elements_limits l
					JOIN budgets_elements e ON e.id = l.element_id
					WHERE e.budget_id = $1 AND e.external_id = $2 AND l.period = '2026-08-01'), '')`, budgetID1, id).Scan(&got)
				if err != nil {
					t.Fatal(err)
				}
				if got != "" {
					got = vo.NewDecimal(got).String()
				}
				if got != w {
					t.Errorf("plan of %s = %q, want %q", id, got, w)
				}
			}
		})
	}
}
