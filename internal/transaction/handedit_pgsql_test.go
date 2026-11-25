//go:build enginecompare

package transaction_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// The import AI job holds the transaction's row while it writes a category
// and payee. A hand edit the user started from the empty form must wait for
// it and then see the AI's values, so the change it makes (back to empty) is
// reported. Without the row lock the edit reads the empty row, waits only at
// its save, writes over the AI's values and reports nothing. The race needs a
// second connection: every other suite pins its database to one.
func TestHandEditWaitsForAConcurrentAIWrite(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "postgresql" {
		t.Skipf("the race needs two real connections; engine is %q (run with DBTEST_ENGINE=pgsql)", db.Engine)
	}
	ctx := context.Background()
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	account := f.Account(fixture.Account{UserID: userID})
	category := f.Category(fixture.Category{UserID: userID, Type: 0})
	payee := f.Payee(fixture.Payee{UserID: userID})
	svc := newWriteService(t, db)
	rec := &recordingReclassifications{}
	svc.SetReclassificationListener(rec)
	uid := mustID(t, userID)
	created, err := svc.CreateTransaction(ctx, uid, model.CreateTransactionRequest{
		Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("10.00"), AccountId: account, Date: "2024-04-01 00:00:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	txID := created.Item.Id

	var schema string
	if err := db.Raw.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read current_schema: %v", err)
	}
	raw2, err := pgsql.OpenDB(os.Getenv(dbtest.PgsqlURLEnv))
	if err != nil {
		t.Fatalf("open second pool: %v", err)
	}
	t.Cleanup(func() { _ = raw2.Close() })
	raw2.SetMaxOpenConns(1)
	if _, err := raw2.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %q`, schema)); err != nil {
		t.Fatalf("set search_path on the second pool: %v", err)
	}
	tx2 := backend.NewTxManager(raw2)

	locked, release := make(chan struct{}), make(chan struct{})
	aiDone := make(chan error, 1)
	go func() {
		aiDone <- tx2.WithTx(ctx, func(ctx context.Context) error {
			q := tx2.Querier(ctx)
			if _, err := q.ExecContext(ctx, `SELECT id FROM transactions WHERE id = $1 FOR UPDATE`, txID); err != nil {
				return err
			}
			if _, err := q.ExecContext(ctx, `UPDATE transactions SET category_id = $1, payee_id = $2 WHERE id = $3`, category, payee, txID); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-aiDone:
		t.Fatalf("the AI write ended before holding the row: %v", err)
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the AI write never took the row")
	}

	edited := make(chan error, 1)
	go func() {
		_, err := svc.UpdateTransaction(ctx, uid, model.UpdateTransactionRequest{
			Id: txID, Type: "expense", Amount: vo.NewFlexString("10.00"), AccountId: account, Date: "2024-04-01 00:00:00",
		})
		edited <- err
	}()
	// Never t.Fatal while the AI write is parked: the cleanup DROP SCHEMA would
	// wait on its lock. Release first, assert after.
	select {
	case err := <-edited:
		edited <- err
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-aiDone; err != nil {
		t.Fatalf("AI write: %v", err)
	}
	select {
	case err := <-edited:
		if err != nil {
			t.Fatalf("hand edit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the hand edit never completed")
	}
	if len(rec.ids) != 1 || rec.ids[0] != txID {
		t.Fatalf("reported = %v, want the edit that emptied the AI's category and payee reported", rec.ids)
	}
}
