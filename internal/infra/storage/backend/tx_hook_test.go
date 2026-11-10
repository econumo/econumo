package backend_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestBeforeCommit_RunsOnceBeforeOutermostCommit(t *testing.T) {
	db := dbtest.New(t)
	m := db.TX
	calls := 0
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			if !m.BeforeCommit(ctx, "k", func(context.Context) error { calls++; return nil }) {
				t.Fatal("expected inTx")
			}
		}
		return m.WithTx(ctx, func(ctx context.Context) error {
			m.BeforeCommit(ctx, "k", func(context.Context) error { calls++; return nil })
			if calls != 0 {
				t.Fatal("hook ran before commit")
			}
			return nil
		})
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestBeforeCommit_RunsInRegistrationOrderInsideTx(t *testing.T) {
	db := dbtest.New(t)
	m := db.TX
	var order []string
	var seen int
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		if _, err := m.Querier(ctx).ExecContext(ctx, db.Rebind("INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'ZZY', 'z', 2, '2024-01-01 00:00:00')"), uuid.NewString()); err != nil {
			return err
		}
		m.BeforeCommit(ctx, "a", func(ctx context.Context) error {
			order = append(order, "a")
			return m.Querier(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies WHERE code = 'ZZY'").Scan(&seen)
		})
		m.BeforeCommit(ctx, "b", func(context.Context) error { order = append(order, "b"); return nil })
		return nil
	})
	if err != nil || len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("err=%v order=%v", err, order)
	}
	if seen != 1 {
		t.Fatalf("hook did not run inside the tx: saw %d uncommitted rows", seen)
	}
}

func TestBeforeCommit_ErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	m := db.TX
	boom := errors.New("boom")
	id := uuid.NewString()
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		if _, err := m.Querier(ctx).ExecContext(ctx, db.Rebind("INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'ZZZ', 'z', 2, '2024-01-01 00:00:00')"), id); err != nil {
			return err
		}
		m.BeforeCommit(ctx, "k", func(context.Context) error { return boom })
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v, want boom", err)
	}
	var n int
	if err := db.Raw.QueryRow(db.Rebind("SELECT COUNT(*) FROM currencies WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("write survived a failed hook: %d rows", n)
	}
}

func TestBeforeCommit_NotInTx(t *testing.T) {
	m := dbtest.New(t).TX
	called := false
	if m.BeforeCommit(context.Background(), "k", func(context.Context) error { called = true; return nil }) {
		t.Fatal("expected inTx=false")
	}
	if called {
		t.Fatal("fn must not run outside a tx")
	}
}

func TestBeforeCommit_SkippedOnRollback(t *testing.T) {
	m := dbtest.New(t).TX
	called := false
	boom := errors.New("boom")
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		m.BeforeCommit(ctx, "k", func(context.Context) error { called = true; return nil })
		return boom
	})
	if !errors.Is(err, boom) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestTxValue_SharedAcrossSavepoints(t *testing.T) {
	m := dbtest.New(t).TX
	type key struct{}
	if v, in := m.TxValue(context.Background(), key{}, func() any { return new(int) }); v != nil || in {
		t.Fatalf("outside tx: v=%v in=%v", v, in)
	}
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		outer, in := m.TxValue(ctx, key{}, func() any { return new(int) })
		if !in || outer == nil {
			t.Fatal("expected a value inside tx")
		}
		return m.WithTx(ctx, func(ctx context.Context) error {
			inner, _ := m.TxValue(ctx, key{}, func() any { t.Fatal("init ran twice"); return nil })
			if inner != outer {
				t.Fatal("nested WithTx must share the value")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCommitHookRunsInsideTheTransaction(t *testing.T) {
	db := dbtest.New(t)
	m := db.TX
	var seen int
	var order []string
	ctx := backend.WithCommitHook(context.Background(), func(ctx context.Context) error {
		order = append(order, "commit-hook")
		return m.Querier(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies WHERE code = 'ZZH'").Scan(&seen)
	})
	err := m.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.Querier(ctx).ExecContext(ctx, db.Rebind("INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'ZZH', 'z', 2, '2024-01-01 00:00:00')"), uuid.NewString()); err != nil {
			return err
		}
		m.BeforeCommit(ctx, "k", func(context.Context) error { order = append(order, "before-commit"); return nil })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("hook did not see the uncommitted row: saw %d", seen)
	}
	if len(order) != 2 || order[0] != "commit-hook" || order[1] != "before-commit" {
		t.Fatalf("order=%v", order)
	}
	if !backend.CommitHookDone(ctx) {
		t.Fatal("CommitHookDone=false after a commit")
	}
}

func TestCommitHookErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	m := db.TX
	boom := errors.New("boom")
	id := uuid.NewString()
	ctx := backend.WithCommitHook(context.Background(), func(context.Context) error { return boom })
	err := m.WithTx(ctx, func(ctx context.Context) error {
		_, err := m.Querier(ctx).ExecContext(ctx, db.Rebind("INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'ZZI', 'z', 2, '2024-01-01 00:00:00')"), id)
		return err
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v, want boom", err)
	}
	var n int
	if err := db.Raw.QueryRow(db.Rebind("SELECT COUNT(*) FROM currencies WHERE id = ?"), id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("write survived a failed hook: %d rows", n)
	}
	if backend.CommitHookDone(ctx) {
		t.Fatal("CommitHookDone=true after a rollback")
	}
}

func TestCommitHookFiresOncePerContextAcrossTopLevelTransactions(t *testing.T) {
	m := dbtest.New(t).TX
	calls := 0
	ctx := backend.WithCommitHook(context.Background(), func(context.Context) error { calls++; return nil })
	for range 2 {
		// A derived context, as a request handler's own would be, and a nested
		// WithTx, which must not fire it either.
		derived := context.WithValue(ctx, struct{}{}, 1)
		if err := m.WithTx(derived, func(ctx context.Context) error {
			return m.WithTx(ctx, func(context.Context) error { return nil })
		}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
}

func TestCommitHookFiresAgainAfterItsTransactionRolledBack(t *testing.T) {
	m := dbtest.New(t).TX
	boom := errors.New("boom")
	calls := 0
	ctx := backend.WithCommitHook(context.Background(), func(context.Context) error { calls++; return nil })
	if err := m.WithTx(ctx, func(ctx context.Context) error {
		m.BeforeCommit(ctx, "k", func(context.Context) error { return boom })
		return nil
	}); !errors.Is(err, boom) {
		t.Fatalf("err=%v, want boom", err)
	}
	if backend.CommitHookDone(ctx) {
		t.Fatal("CommitHookDone=true after a rollback")
	}
	if err := m.WithTx(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !backend.CommitHookDone(ctx) {
		t.Fatalf("calls=%d done=%v, want 2 and true", calls, backend.CommitHookDone(ctx))
	}
}

func TestCommitHookSkippedWhenTheUnitOfWorkFails(t *testing.T) {
	m := dbtest.New(t).TX
	boom := errors.New("boom")
	calls := 0
	ctx := backend.WithCommitHook(context.Background(), func(context.Context) error { calls++; return nil })
	if err := m.WithTx(ctx, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err=%v, want boom", err)
	}
	if calls != 0 {
		t.Fatalf("calls=%d, want 0", calls)
	}
}
