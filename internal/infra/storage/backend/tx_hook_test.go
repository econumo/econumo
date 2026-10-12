package backend_test

import (
	"context"
	"errors"
	"testing"

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
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		m.BeforeCommit(ctx, "a", func(ctx context.Context) error {
			order = append(order, "a")
			_, err := m.Querier(ctx).ExecContext(ctx, "SELECT 1")
			return err
		})
		m.BeforeCommit(ctx, "b", func(context.Context) error { order = append(order, "b"); return nil })
		return nil
	})
	if err != nil || len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("err=%v order=%v", err, order)
	}
}

func TestBeforeCommit_ErrorRollsBack(t *testing.T) {
	db := dbtest.New(t)
	m := db.TX
	boom := errors.New("boom")
	err := m.WithTx(context.Background(), func(ctx context.Context) error {
		if _, err := m.Querier(ctx).ExecContext(ctx, db.Rebind("INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'ZZZ', 'z', 2, '2024-01-01 00:00:00')"), "cur-x"); err != nil {
			return err
		}
		m.BeforeCommit(ctx, "k", func(context.Context) error { return boom })
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v, want boom", err)
	}
	var n int
	if err := db.Raw.QueryRow("SELECT COUNT(*) FROM currencies WHERE id = 'cur-x'").Scan(&n); err != nil {
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
