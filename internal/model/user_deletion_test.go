package model

import (
	"slices"
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

func TestInactivityMonths(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	with := func(value *string) *User {
		u := NewUser(vo.NewId(), "a@b.test", "A", "face:fuchsia", "h", "s", now)
		if value != nil {
			u.Options = append(u.Options, NewUserOption(vo.NewId(), OptionInactivityMonths, value, now))
		}
		return u
	}
	s := func(v string) *string { return &v }
	for _, c := range []struct {
		name   string
		value  *string
		server int
		want   int
	}{
		{"absent", nil, 12, 12},
		{"six months", s("6"), 12, 6},
		{"seeded lifetime", s("36"), 12, 36},
		{"zero", s("0"), 12, 12},
		{"negative", s("-1"), 12, 12},
		{"garbage", s("abc"), 12, 12},
		{"empty", s(""), 12, 12},
		{"server off, option set", s("6"), 0, 6},
	} {
		if got := with(c.value).InactivityMonths(c.server); got != c.want {
			t.Errorf("%s: InactivityMonths = %d, want %d", c.name, got, c.want)
		}
		cand := InactiveCandidate{InactivityOption: c.value}
		if got := cand.InactivityMonths(c.server); got != c.want {
			t.Errorf("%s: candidate InactivityMonths = %d, want %d", c.name, got, c.want)
		}
	}
	u := with(nil)
	u.Options = append(u.Options, NewUserOption(vo.NewId(), OptionInactivityMonths, nil, now))
	if got := u.InactivityMonths(12); got != 12 {
		t.Errorf("NULL value: InactivityMonths = %d, want 12", got)
	}
}

func TestInactivityChoices(t *testing.T) {
	for server, want := range map[int][]int{12: {6, 12, 24}, 3: {1, 3, 6}, 1: {1, 2}, 0: {}} {
		got := InactivityChoices(server)
		if got == nil || !slices.Equal(got, want) {
			t.Errorf("InactivityChoices(%d) = %#v, want %v", server, got, want)
		}
	}
}

func TestSetInactivityMonths(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	u := NewUser(vo.NewId(), "a@b.test", "A", "face:fuchsia", "h", "s", now)
	first := vo.NewId()
	u.SetInactivityMonths(6, first, now)
	o := u.Option(OptionInactivityMonths)
	if o == nil || o.Value == nil || *o.Value != "6" || o.ID != first {
		t.Fatalf("option = %+v, want 6 with the given id", o)
	}
	u.SetInactivityMonths(24, vo.NewId(), now.Add(time.Hour))
	if n := len(u.Options); n != 1 {
		t.Fatalf("options = %d, want the row updated in place", n)
	}
	if o := u.Option(OptionInactivityMonths); *o.Value != "24" || o.ID != first {
		t.Fatalf("option = %+v, want 24 on the first row", o)
	}
}
