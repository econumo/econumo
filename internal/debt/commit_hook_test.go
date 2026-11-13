package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// hookRecorder stands in for the Idempotency-Key middleware: its hook runs in
// the first top-level transaction that commits under the request's context.
type hookRecorder struct {
	fired int
	// sawWrite reports, inside the hooked transaction, whether the write is in
	sawWrite bool
}

func (h *hookRecorder) ctx(check func(ctx context.Context) bool) context.Context {
	return backend.WithCommitHook(context.Background(), func(ctx context.Context) error {
		h.fired++
		h.sawWrite = check(ctx)
		return nil
	})
}

func (e *env) snapshotCount(ctx context.Context, debtID string) int {
	got, err := e.repo.Snapshots(ctx, []vo.Id{vo.MustParseId(debtID)})
	if err != nil {
		return -1
	}
	return len(got)
}

// Every debt write runs its sync and its write in one transaction, so the
// request's commit hook (the Idempotency-Key) fires once, in the transaction
// that carries the write: a retry can never be told "already done" before it
// is, and a crash between two commits cannot strand the key.
func TestWrites_CommitHookFiresOnceWithTheWrite(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error
	}{
		{"create", func(t *testing.T, e *env, _ *model.DebtResult, h *hookRecorder) error {
			other := e.newCategory(e.user, true)
			req := e.createReq()
			req.Name, req.Rules = "Second", []model.DebtRuleRequest{rule(other)}
			ctx := h.ctx(func(ctx context.Context) bool {
				debts, err := e.repo.ListByBudget(ctx, vo.MustParseId(e.budget))
				return err == nil && len(debts) == 2
			})
			_, err := e.svc.CreateDebt(ctx, e.uid(), req)
			return err
		}},
		{"update", func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error {
			req := e.updateReq(res)
			req.Name = "Renamed"
			ctx := h.ctx(func(ctx context.Context) bool {
				d, err := e.repo.GetByID(ctx, vo.MustParseId(res.Id))
				return err == nil && d.Name == "Renamed"
			})
			_, err := e.svc.UpdateDebt(ctx, e.uid(), req)
			return err
		}},
		{"delete", func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error {
			ctx := h.ctx(func(ctx context.Context) bool {
				_, err := e.repo.GetByID(ctx, vo.MustParseId(res.Id))
				return err != nil
			})
			_, err := e.svc.DeleteDebt(ctx, e.uid(), model.DeleteDebtRequest{Id: res.Id, ConfirmDataRemoval: true})
			return err
		}},
		{"add-debt-rule", func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error {
			other := e.newCategory(e.user, true)
			ctx := h.ctx(func(ctx context.Context) bool {
				d, err := e.repo.GetByID(ctx, vo.MustParseId(res.Id))
				return err == nil && len(d.Rules) == 2
			})
			_, err := e.svc.AddDebtRule(ctx, e.uid(), model.AddDebtRuleRequest{Id: res.Id, Rule: rule(other)})
			return err
		}},
		{"reconcile", func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error {
			ctx := h.ctx(func(ctx context.Context) bool { return e.snapshotCount(ctx, res.Id) == 2 })
			_, err := e.svc.ReconcileDebt(ctx, e.uid(), model.ReconcileDebtRequest{Id: res.Id, Balance: vo.NewFlexString("9000")})
			return err
		}},
		{"delete-reconciliation", func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error {
			if _, err := e.svc.ReconcileDebt(context.Background(), e.uid(), model.ReconcileDebtRequest{Id: res.Id, Balance: vo.NewFlexString("9000")}); err != nil {
				t.Fatal(err)
			}
			snaps := e.snapshots(t, res.Id)
			ctx := h.ctx(func(ctx context.Context) bool { return e.snapshotCount(ctx, res.Id) == 1 })
			_, err := e.svc.DeleteDebtReconciliation(ctx, e.uid(), model.DeleteDebtReconciliationRequest{Id: res.Id, ReconciliationId: snaps[1].ID.String()})
			return err
		}},
		{"mark-paid-off", func(t *testing.T, e *env, res *model.DebtResult, h *hookRecorder) error {
			ctx := h.ctx(func(ctx context.Context) bool { return e.snapshotCount(ctx, res.Id) == 2 })
			_, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			res := e.create(t, func(r *model.CreateDebtRequest) {
				r.AmountBorrowed, r.StartDate, r.LastPaymentDate = vo.NewFlexString("10000"), ptr("2026-01-10"), "2026-12-10"
			})
			// a payment the write's sync splits first
			e.expense(t, e.account, ymd(2026, 2, 10), "1000", "")
			h := &hookRecorder{}
			if err := c.run(t, e, res, h); err != nil {
				t.Fatal(err)
			}
			if h.fired != 1 || !h.sawWrite {
				t.Fatalf("hook fired %d times, saw the write: %v", h.fired, h.sawWrite)
			}
		})
	}
}

// A write refused after its sync records no key: nothing committed.
func TestWrites_RefusedAfterTheSyncRecordNoKey(t *testing.T) {
	cases := []struct {
		name string
		run  func(e *env, res *model.DebtResult, ctx context.Context) error
	}{
		{"update", func(e *env, res *model.DebtResult, ctx context.Context) error {
			eur := e.eur()
			e.currency.unusable[eur] = true
			req := e.updateReq(res)
			req.CurrencyId = eur
			_, err := e.svc.UpdateDebt(ctx, e.uid(), req)
			return err
		}},
		{"reconcile", func(e *env, res *model.DebtResult, ctx context.Context) error {
			_, err := e.svc.ReconcileDebt(ctx, e.uid(), model.ReconcileDebtRequest{Id: res.Id, Balance: vo.NewFlexString("9000"), BalanceDate: "2026-01-05"})
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			res := e.create(t, func(r *model.CreateDebtRequest) {
				r.AmountBorrowed, r.StartDate, r.LastPaymentDate = vo.NewFlexString("10000"), ptr("2026-01-10"), "2026-12-10"
			})
			pay := e.expense(t, e.account, ymd(2026, 2, 10), "1000", "")
			h := &hookRecorder{}
			ctx := h.ctx(func(context.Context) bool { return true })
			if err := c.run(e, res, ctx); err == nil {
				t.Fatal("the write was not refused")
			}
			if h.fired != 0 || backend.CommitHookDone(ctx) {
				t.Fatalf("hook fired %d times, recorded: %v", h.fired, backend.CommitHookDone(ctx))
			}
			// the sync's splits went with the refused write
			if _, ok := e.splits(t, res.Id)[pay]; ok {
				t.Fatalf("splits = %v", e.splits(t, res.Id))
			}
		})
	}
}
