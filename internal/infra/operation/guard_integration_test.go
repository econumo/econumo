package operation_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func TestGuard_Claim_Idempotency(t *testing.T) {
	db := dbtest.NewSQLite(t)
	guard := operation.NewGuard("sqlite", db.TX)
	ctx := context.Background()
	id := vo.NewId()

	already, err := guard.Claim(ctx, id, fixedTime)
	if err != nil {
		t.Fatalf("Claim first: %v", err)
	}
	if already {
		t.Error("first Claim should be already=false")
	}

	already, err = guard.Claim(ctx, id, fixedTime)
	if err != nil {
		t.Fatalf("Claim second: %v", err)
	}
	if !already {
		t.Error("second Claim should be already=true")
	}

	// A different id is independent.
	already, err = guard.Claim(ctx, vo.NewId(), fixedTime)
	if err != nil {
		t.Fatalf("Claim other: %v", err)
	}
	if already {
		t.Error("Claim of a fresh id should be already=false")
	}
}

func TestGuard_MarkHandled(t *testing.T) {
	db := dbtest.NewSQLite(t)
	guard := operation.NewGuard("sqlite", db.TX)
	ctx := context.Background()
	id := vo.NewId()

	if _, err := guard.Claim(ctx, id, fixedTime); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	var handled bool
	if err := db.Raw.QueryRowContext(ctx, `SELECT is_handled FROM operation_requests_ids WHERE id = ?`, id.String()).Scan(&handled); err != nil {
		t.Fatalf("read before: %v", err)
	}
	if handled {
		t.Fatal("is_handled should start false")
	}

	later := fixedTime.Add(time.Hour)
	if err := guard.MarkHandled(ctx, id, later); err != nil {
		t.Fatalf("MarkHandled: %v", err)
	}
	var updatedAt time.Time
	if err := db.Raw.QueryRowContext(ctx, `SELECT is_handled, updated_at FROM operation_requests_ids WHERE id = ?`, id.String()).Scan(&handled, &updatedAt); err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !handled {
		t.Error("is_handled should be true after MarkHandled")
	}
	if !updatedAt.Equal(later) {
		t.Errorf("updated_at should advance to %v, got %v", later, updatedAt)
	}
}
