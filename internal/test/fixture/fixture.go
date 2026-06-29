// Package fixture is the single, typed, engine-portable way tests seed database
// rows. It centralizes the cross-engine gotchas in one place: the `?`-vs-`$N`
// placeholder difference between SQLite and PostgreSQL, BOOLEAN columns rejecting
// integer 1/0 on Postgres, and time.Time values needing to be bound as a bare
// "Y-m-d H:i:s" string (the sqlite driver serializes time.Time to RFC3339, which
// SQLite's datetime() cannot parse).
//
// All of that is handled in ONE place here, so a test reads as intent:
//
//	f := fixture.New(t, db)
//	owner := f.User(fixture.User{Email: "owner@example.test"})
//	acct := f.Account(fixture.Account{UserID: owner, Name: "Cash"})
//	f.Transaction(fixture.Transaction{UserID: owner, AccountID: acct, Amount: "12.50"})
//
// Every builder method inserts immediately and returns the row's id (a string),
// so later rows reference earlier ones. Unset fields fall back to sensible
// defaults; ids default to a fresh UUID. Timestamps default to a fixed,
// monotonically increasing instant so insertion order is deterministic without
// the caller passing times.
package fixture

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// Builder seeds rows into a single test database. Construct with New. Not safe
// for concurrent use (tests seed serially).
type Builder struct {
	t      testing.TB
	db     *dbtest.DB
	clock  time.Time
	step   time.Duration
	encode *auth.EncodeService // optional; enables real encrypted email + identifier
	hasher *auth.PasswordHasher
}

// New returns a Builder for db. Seeded rows get fixed, increasing timestamps so
// ordering is deterministic. Users are seeded with literal placeholder
// identifier/email/password unless WithCrypto is set (which logins need).
func New(t testing.TB, db *dbtest.DB) *Builder {
	t.Helper()
	return &Builder{
		t:     t,
		db:    db,
		clock: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC),
		step:  time.Second,
	}
}

// WithCrypto enables real password hashing + email encryption for User(), so the
// seeded user can authenticate through the login endpoint. dataSalt must be the
// SAME salt the harness configures on the EncodeService (AES-128: 16 bytes).
func (b *Builder) WithCrypto(dataSalt string) *Builder {
	b.encode = auth.NewEncodeService(dataSalt)
	b.hasher = auth.NewPasswordHasher()
	return b
}

// At pins the next default timestamp. Subsequent rows continue stepping from
// here. Most tests never need this; it exists for tests asserting on time.
func (b *Builder) At(ts time.Time) *Builder {
	b.clock = ts
	return b
}

// now returns the current default timestamp and advances the clock so each
// seeded row gets a distinct, increasing created_at (deterministic ordering).
func (b *Builder) now() time.Time {
	ts := b.clock
	b.clock = b.clock.Add(b.step)
	return ts
}

// insert runs an INSERT with engine-portable placeholders, converting any
// time.Time arg to the bare "Y-m-d H:i:s" string the production code stores.
// Boolean columns must use TRUE/FALSE literals IN THE QUERY TEXT (not bound
// ints) — see the package doc. Fails the test on error.
func (b *Builder) insert(query string, args ...any) {
	b.t.Helper()
	out := make([]any, len(args))
	for i, a := range args {
		if tm, ok := a.(time.Time); ok {
			out[i] = tm.Format("2006-01-02 15:04:05")
		} else {
			out[i] = a
		}
	}
	if _, err := b.db.Raw.ExecContext(context.Background(), rebind(b.db.Engine, query), out...); err != nil {
		b.t.Fatalf("fixture insert (%s) %q: %v", b.db.Engine, query, err)
	}
}

// rebind converts ?-style placeholders to $N for PostgreSQL; SQLite keeps ?.
func rebind(engine, query string) string {
	if engine != "postgresql" {
		return query
	}
	var sb strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(n))
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

// orNewID returns id if non-empty, else a fresh UUID.
func (b *Builder) orNewID(id string) string {
	if id != "" {
		return id
	}
	return NewID()
}
