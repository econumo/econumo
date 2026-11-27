package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func TestEmailHash(t *testing.T) {
	if got, want := EmailHash("  Anna@Example.TEST "), EmailHash("anna@example.test"); got != want {
		t.Fatalf("EmailHash is not normalized: %s vs %s", got, want)
	}
	if got := EmailHash("a@b.test"); len(got) != 64 {
		t.Fatalf("EmailHash length = %d, want 64", len(got))
	}
	if got := EmailHash("A@B.test"); got != "d98fb7ed563d6a2727d2de61d3ec97dbdf76dbc88916a54cefb32e3f54d531fa" {
		t.Fatalf("EmailHash(A@B.test) = %s, want lowercase hex sha256 of a@b.test", got)
	}
}

func TestDeletionRequestExpiry(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	id, userID := vo.NewId(), vo.NewId()
	r := NewDeletionRequest(id, userID, "a@b.test", "hash", now)
	if r.ID != id || r.UserID != userID || r.Email != "a@b.test" || r.Code != "hash" || !r.CreatedAt.Equal(now) {
		t.Fatalf("request = %+v", r)
	}
	if !r.ExpiredAt.Equal(now.Add(DeletionRequestTTL)) {
		t.Fatalf("expired at %v, want %v", r.ExpiredAt, now.Add(DeletionRequestTTL))
	}
	if r.IsExpired(now.Add(DeletionRequestTTL)) {
		t.Fatal("expired at exactly the TTL")
	}
	if !r.IsExpired(now.Add(DeletionRequestTTL + time.Second)) {
		t.Fatal("not expired after the TTL")
	}
}
