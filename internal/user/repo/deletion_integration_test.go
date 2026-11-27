package repo_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

var deletionNow = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

func setLastActive(t *testing.T, db *dbtest.DB, userID string, at *time.Time) {
	t.Helper()
	db.Exec(t, db.Rebind(`UPDATE users SET last_active_at = ? WHERE id = ?`), at, userID)
}

func setCreatedAt(t *testing.T, db *dbtest.DB, userID string, at time.Time) {
	t.Helper()
	db.Exec(t, db.Rebind(`UPDATE users SET created_at = ? WHERE id = ?`), at, userID)
}

// seedUser gives every user a distinct email: the fixture's default derives it
// from the first 8 id characters, which UUIDv7 ids minted in one millisecond share.
func seedUser(f *fixture.Builder, u fixture.User) string {
	if u.Email == "" {
		u.Email = "user-" + fixture.NewID() + "@example.test"
	}
	return f.User(u)
}

func ago(d time.Duration) *time.Time {
	ts := deletionNow.Add(-d)
	return &ts
}

func ids(t *testing.T, raw ...string) []vo.Id {
	t.Helper()
	out := make([]vo.Id, 0, len(raw))
	for _, s := range raw {
		out = append(out, vo.MustParseId(s))
	}
	slices.SortFunc(out, func(a, b vo.Id) int {
		switch {
		case a.String() < b.String():
			return -1
		case a.String() > b.String():
			return 1
		}
		return 0
	})
	return out
}

func TestDeletionRepo_MarkAndUnmark(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	ctx := context.Background()
	u := vo.MustParseId(seedUser(fixture.New(t, db), fixture.User{}))

	if n, err := r.Mark(ctx, u, model.DeletionReasonSelf, deletionNow); err != nil || n != 1 {
		t.Fatalf("first Mark = %d, %v; want 1, nil", n, err)
	}
	if n, err := r.Mark(ctx, u, model.DeletionReasonInactive, deletionNow.Add(time.Hour)); err != nil || n != 0 {
		t.Fatalf("second Mark = %d, %v; want 0, nil", n, err)
	}
	st, err := r.GetState(ctx, u)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.MarkedAt == nil || !st.MarkedAt.Equal(deletionNow) || st.Reason != model.DeletionReasonSelf {
		t.Fatalf("state after Mark = %+v; want marked at %v by %q", st, deletionNow, model.DeletionReasonSelf)
	}
	if !st.CreatedAt.Equal(fixedTime) {
		t.Errorf("CreatedAt = %v, want %v", st.CreatedAt, fixedTime)
	}

	if n, err := r.Unmark(ctx, u); err != nil || n != 1 {
		t.Fatalf("Unmark = %d, %v; want 1, nil", n, err)
	}
	st, err = r.GetState(ctx, u)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.MarkedAt != nil || st.Reason != "" {
		t.Fatalf("state after Unmark = %+v; want both columns cleared", st)
	}
	if n, err := r.Unmark(ctx, u); err != nil || n != 0 {
		t.Fatalf("Unmark of an unmarked row = %d, %v; want 0, nil", n, err)
	}

	if _, err := r.GetState(ctx, vo.NewId()); err == nil {
		t.Fatal("GetState on a missing user must fail")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("GetState on a missing user: want NotFound, got %v", err)
	}
}

func TestDeletionRepo_TouchLastActive(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	ctx := context.Background()
	raw := seedUser(fixture.New(t, db), fixture.User{})
	u := vo.MustParseId(raw)

	stamp := func() *time.Time {
		t.Helper()
		st, err := r.GetState(ctx, u)
		if err != nil {
			t.Fatalf("GetState: %v", err)
		}
		return st.LastActiveAt
	}

	setLastActive(t, db, raw, nil)
	if err := r.TouchLastActive(ctx, u, deletionNow); err != nil {
		t.Fatalf("TouchLastActive: %v", err)
	}
	if got := stamp(); got == nil || !got.Equal(deletionNow) {
		t.Fatalf("NULL stamp after touch = %v, want %v", got, deletionNow)
	}

	setLastActive(t, db, raw, ago(23*time.Hour))
	if err := r.TouchLastActive(ctx, u, deletionNow); err != nil {
		t.Fatalf("TouchLastActive: %v", err)
	}
	if got := stamp(); got == nil || !got.Equal(*ago(23 * time.Hour)) {
		t.Fatalf("23h-old stamp after touch = %v, want it unchanged at %v", got, *ago(23 * time.Hour))
	}

	setLastActive(t, db, raw, ago(25*time.Hour))
	if err := r.TouchLastActive(ctx, u, deletionNow); err != nil {
		t.Fatalf("TouchLastActive: %v", err)
	}
	if got := stamp(); got == nil || !got.Equal(deletionNow) {
		t.Fatalf("25h-old stamp after touch = %v, want %v", got, deletionNow)
	}
}

func TestDeletionRepo_ListMarked(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	ctx := context.Background()
	f := fixture.New(t, db)
	later := f.User(fixture.User{Email: "later@example.test"})
	earlier := f.User(fixture.User{Email: "earlier@example.test"})
	seedUser(f, fixture.User{})

	if _, err := r.Mark(ctx, vo.MustParseId(later), model.DeletionReasonCLI, deletionNow.Add(time.Hour)); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	if _, err := r.Mark(ctx, vo.MustParseId(earlier), model.DeletionReasonInactive, deletionNow); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	got, err := r.ListMarked(ctx)
	if err != nil {
		t.Fatalf("ListMarked: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListMarked returned %d rows, want 2: %+v", len(got), got)
	}
	if got[0].ID.String() != earlier || got[0].Email != "earlier@example.test" || got[0].Reason != model.DeletionReasonInactive || !got[0].MarkedAt.Equal(deletionNow) {
		t.Errorf("first marked = %+v", got[0])
	}
	if got[1].ID.String() != later || got[1].Reason != model.DeletionReasonCLI {
		t.Errorf("second marked = %+v", got[1])
	}
}

func TestDeletionRepo_InactiveCandidatesAndActiveConnection(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	ctx := context.Background()
	f := fixture.New(t, db)
	cutoff := deletionNow.Add(-31 * 24 * time.Hour)
	old := ago(60 * 24 * time.Hour)
	fresh := ago(24 * time.Hour)

	stale := seedUser(f, fixture.User{Name: "Stale"})
	setLastActive(t, db, stale, old)

	marked := seedUser(f, fixture.User{})
	setLastActive(t, db, marked, old)
	if _, err := r.Mark(ctx, vo.MustParseId(marked), model.DeletionReasonSelf, deletionNow); err != nil {
		t.Fatalf("Mark: %v", err)
	}

	deactivated := seedUser(f, fixture.User{Inactive: true})
	setLastActive(t, db, deactivated, old)

	recent := seedUser(f, fixture.User{})
	setLastActive(t, db, recent, fresh)

	newNull := seedUser(f, fixture.User{})
	setLastActive(t, db, newNull, nil)
	setCreatedAt(t, db, newNull, *fresh)

	oldNull := seedUser(f, fixture.User{})
	setLastActive(t, db, oldNull, nil)

	connected := seedUser(f, fixture.User{})
	setLastActive(t, db, connected, old)
	activePeer := seedUser(f, fixture.User{})
	setLastActive(t, db, activePeer, fresh)
	f.Connect(connected, activePeer)

	connectedToMarked := seedUser(f, fixture.User{})
	setLastActive(t, db, connectedToMarked, old)
	markedPeer := seedUser(f, fixture.User{})
	setLastActive(t, db, markedPeer, fresh)
	if _, err := r.Mark(ctx, vo.MustParseId(markedPeer), model.DeletionReasonSelf, deletionNow); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	f.Connect(connectedToMarked, markedPeer)

	got, err := r.ListInactiveCandidates(ctx, cutoff)
	if err != nil {
		t.Fatalf("ListInactiveCandidates: %v", err)
	}
	gotIDs := make([]vo.Id, 0, len(got))
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}
	want := ids(t, stale, oldNull, connectedToMarked)
	if !slices.Equal(gotIDs, want) {
		t.Fatalf("candidates = %v, want %v (by id)", gotIDs, want)
	}
	for _, c := range got {
		if c.ID.String() == stale && (c.Name != "Stale" || c.Email == "") {
			t.Errorf("candidate row = %+v, want name and stored email", c)
		}
	}

	for _, tc := range []struct {
		name string
		id   string
		want bool
	}{
		{"connected to an active user", connected, true},
		{"connected to a marked user", connectedToMarked, false},
		{"no connections", stale, false},
	} {
		has, err := r.HasActiveConnection(ctx, vo.MustParseId(tc.id), cutoff)
		if err != nil {
			t.Fatalf("%s: HasActiveConnection: %v", tc.name, err)
		}
		if has != tc.want {
			t.Errorf("%s: HasActiveConnection = %v, want %v", tc.name, has, tc.want)
		}
	}
}

func TestDeletionRepo_ConnectedUserIDs(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	ctx := context.Background()
	f := fixture.New(t, db)
	u := seedUser(f, fixture.User{})
	c := seedUser(f, fixture.User{})
	a := seedUser(f, fixture.User{})
	lonely := seedUser(f, fixture.User{})
	f.Connect(u, c)
	f.Connect(u, a)

	got, err := r.ConnectedUserIDs(ctx, vo.MustParseId(u))
	if err != nil {
		t.Fatalf("ConnectedUserIDs: %v", err)
	}
	if want := ids(t, a, c); !slices.Equal(got, want) {
		t.Fatalf("ConnectedUserIDs = %v, want %v", got, want)
	}
	got, err = r.ConnectedUserIDs(ctx, vo.MustParseId(lonely))
	if err != nil {
		t.Fatalf("ConnectedUserIDs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ConnectedUserIDs of an unconnected user = %v, want empty", got)
	}
}

func TestDeletionRepo_LockRowForWipeOnMissingUser(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	ctx := context.Background()
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error {
		if err := r.LockWipes(ctx); err != nil {
			return err
		}
		return r.LockRowForWipe(ctx, vo.NewId())
	}); err != nil {
		t.Fatalf("LockWipes + LockRowForWipe on a missing user: %v", err)
	}
}

func TestDeletionRepo_DeleteUserKeepsAudit(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewDeletionRepo(db.Engine, db.TX)
	audit := userrepo.NewAuditRepo(db.Engine, db.TX)
	ctx := context.Background()
	u := vo.MustParseId(seedUser(fixture.New(t, db), fixture.User{}))
	hash := model.EmailHash("gone@example.test")

	if _, err := audit.Insert(ctx, model.UserAuditEvent{ID: vo.NewId(), UserID: u, Event: model.AuditDeleted, EmailHash: &hash, Details: `{"reason":"cli"}`, CreatedAt: deletionNow}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if n, err := r.DeleteUser(ctx, u); err != nil || n != 1 {
		t.Fatalf("DeleteUser = %d, %v; want 1, nil", n, err)
	}
	if _, err := r.GetState(ctx, u); err == nil {
		t.Fatal("the user row must be gone")
	}
	rows, err := audit.ListByEmailHash(ctx, hash)
	if err != nil {
		t.Fatalf("ListByEmailHash: %v", err)
	}
	if len(rows) != 1 || rows[0].UserID != u || rows[0].Details != `{"reason":"cli"}` {
		t.Fatalf("audit rows after delete = %+v, want the one row of the deleted user", rows)
	}
	if n, err := r.DeleteUser(ctx, u); err != nil || n != 0 {
		t.Fatalf("second DeleteUser = %d, %v; want 0, nil", n, err)
	}
}

func TestAuditRepo_InsertTruncatesAndRefIDIsUnique(t *testing.T) {
	db := dbtest.New(t)
	audit := userrepo.NewAuditRepo(db.Engine, db.TX)
	ctx := context.Background()
	u := vo.NewId()
	hash := model.EmailHash("someone@example.test")
	ref := vo.NewId()

	first := model.UserAuditEvent{ID: vo.NewId(), UserID: u, Event: model.AuditLifetimeRegranted, EmailHash: &hash, RefID: &ref,
		Details: "{}", CreatedAt: deletionNow.Add(750 * time.Millisecond)}
	if ok, err := audit.Insert(ctx, first); err != nil || !ok {
		t.Fatalf("Insert = %v, %v; want true, nil", ok, err)
	}
	dup := first
	dup.ID = vo.NewId()
	if ok, err := audit.Insert(ctx, dup); err != nil || ok {
		t.Fatalf("Insert with a taken RefID = %v, %v; want false, nil", ok, err)
	}

	rows, err := audit.ListByEmailHash(ctx, hash)
	if err != nil {
		t.Fatalf("ListByEmailHash: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByEmailHash returned %d rows, want 1", len(rows))
	}
	got := rows[0]
	if !got.CreatedAt.Equal(deletionNow) {
		t.Errorf("CreatedAt = %v, want %v (truncated to the second)", got.CreatedAt, deletionNow)
	}
	if got.ID != first.ID || got.Event != model.AuditLifetimeRegranted || got.RefID == nil || *got.RefID != ref || got.EmailHash == nil || *got.EmailHash != hash {
		t.Errorf("round trip = %+v", got)
	}
}

func TestAuditRepo_ListByEventPages(t *testing.T) {
	db := dbtest.New(t)
	audit := userrepo.NewAuditRepo(db.Engine, db.TX)
	ctx := context.Background()

	insert := func(event string, at time.Time) model.UserAuditEvent {
		t.Helper()
		e := model.UserAuditEvent{ID: vo.NewId(), UserID: vo.NewId(), Event: event, Details: "{}", CreatedAt: at}
		if _, err := audit.Insert(ctx, e); err != nil {
			t.Fatalf("Insert: %v", err)
		}
		return e
	}
	insert(model.AuditDeleted, deletionNow.Add(-time.Second))
	insert(model.AuditDeletionMarked, deletionNow)
	same := []model.UserAuditEvent{
		insert(model.AuditDeleted, deletionNow),
		insert(model.AuditDeleted, deletionNow),
		insert(model.AuditDeleted, deletionNow),
	}
	slices.SortFunc(same, func(a, b model.UserAuditEvent) int {
		switch {
		case a.ID.String() < b.ID.String():
			return -1
		case a.ID.String() > b.ID.String():
			return 1
		}
		return 0
	})

	page, err := audit.ListByEvent(ctx, model.AuditDeleted, deletionNow, "", 2)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}
	if len(page) != 2 || page[0].ID != same[0].ID || page[1].ID != same[1].ID {
		t.Fatalf("first page = %+v, want the first two of %v", page, same)
	}
	page, err = audit.ListByEvent(ctx, model.AuditDeleted, page[1].CreatedAt, page[1].ID.String(), 2)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}
	if len(page) != 1 || page[0].ID != same[2].ID {
		t.Fatalf("second page = %+v, want only %v", page, same[2].ID)
	}
}

func TestDeletionRequestRepo(t *testing.T) {
	db := dbtest.New(t)
	users := userrepo.NewRepo(db.Engine, db.TX)
	r := userrepo.NewDeletionRequestRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := deletionNow

	u := model.NewUser(users.NextIdentity(), "stored", "DR", "face:blue", "hash", "salt", now)
	if err := users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("GetByUser on an empty table must fail")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}

	stale := model.NewDeletionRequest(vo.NewId(), u.ID, "stored", "code-hash", now)
	if n, err := r.Save(ctx, stale, u.CredentialsGeneration+1); err != nil || n != 0 {
		t.Fatalf("Save with a stale generation = %d, %v; want 0, nil", n, err)
	}

	req := model.NewDeletionRequest(vo.NewId(), u.ID, "stored", "code-hash", now)
	if n, err := r.Save(ctx, req, u.CredentialsGeneration); err != nil || n != 1 {
		t.Fatalf("Save = %d, %v; want 1, nil", n, err)
	}
	got, err := r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got.ID != req.ID || got.UserID != u.ID || got.Email != "stored" || got.Code != "code-hash" ||
		!got.CreatedAt.Equal(now) || !got.ExpiredAt.Equal(now.Add(model.DeletionRequestTTL)) {
		t.Fatalf("round trip = %+v", got)
	}

	if n, err := r.Consume(ctx, req.ID, u.ID); err != nil || n != 1 {
		t.Fatalf("Consume = %d, %v; want 1, nil", n, err)
	}
	if n, err := r.Consume(ctx, req.ID, u.ID); err != nil || n != 0 {
		t.Fatalf("second Consume = %d, %v; want 0, nil", n, err)
	}

	if _, err := r.Save(ctx, model.NewDeletionRequest(vo.NewId(), u.ID, "stored", "other", now), u.CredentialsGeneration); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := r.DeleteByUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("the deleted request must be gone")
	}
}
