package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const (
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
	optID = "00000000-0000-0000-0000-0000000000a1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

// identA is a realistic 32-char identifier (production identifiers are
// md5(lower(email)) = 32 hex chars); using exactly 32 avoids CHAR(32) padding
// differences between sqlite and Postgres.
const identA = "0123456789abcdef0123456789abcdef"

func newRepos(t *testing.T) (*userrepo.Repo, *userrepo.ReadRepo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	return userrepo.NewRepo(db.Engine, db.TX), userrepo.NewReadRepo(db.Engine, db.TX), db
}

func newTestUser(id vo.Id, identifier, email, name, avatar, password, salt string, isActive bool,
	createdAt, updatedAt time.Time, opts []model.UserOption) *model.User {
	return &model.User{ID: id, Identifier: identifier, Email: email, Name: name, Avatar: avatar,
		Password: password, Salt: salt, IsActive: isActive, CreatedAt: createdAt, UpdatedAt: updatedAt, Options: opts}
}

func TestUserRepo_SaveGetRoundTrip_WithOptions(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	val := "USD"
	opt := model.ReconstituteUserOption(vo.MustParseId(optID), "currency", &val, fixedTime, fixedTime)
	u := newTestUser(
		vo.MustParseId(userA), identA, "enc-email", "Alice", "https://av/a",
		"hash", "salt-a", true, fixedTime, fixedTime, []model.UserOption{opt},
	)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Identifier != identA || got.Email != "enc-email" || got.Name != "Alice" {
		t.Errorf("fields mismatch: %q %q %q", got.Identifier, got.Email, got.Name)
	}
	if got.Avatar != "https://av/a" || got.Password != "hash" || got.Salt != "salt-a" || !got.IsActive {
		t.Errorf("auth/avatar mismatch: %+v", got)
	}
	if !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("createdAt mismatch: %v", got.CreatedAt)
	}
	opts := got.Options
	if len(opts) != 1 || opts[0].Name != "currency" || opts[0].Value == nil || *opts[0].Value != "USD" {
		t.Errorf("options mismatch: %+v", opts)
	}
}

func TestUserRepo_GetByID_NotFound(t *testing.T) {
	repo, _, _ := newRepos(t)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

// TestUserRepo_GetHeaderByID covers the lightweight owner/author-embed lookup:
// it returns id/name/avatar from the user row (no options query) and a
// NotFoundError for a missing id.
func TestUserRepo_GetHeaderByID(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	u := newTestUser(
		vo.MustParseId(userA), identA, "enc-email", "Alice", "https://av/a",
		"hash", "salt-a", true, fixedTime, fixedTime, nil,
	)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h, err := repo.GetHeaderByID(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetHeaderByID: %v", err)
	}
	if h.ID != userA || h.Name != "Alice" || h.Avatar != "https://av/a" {
		t.Errorf("header mismatch: %+v", h)
	}

	_, err = repo.GetHeaderByID(ctx, vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError for missing id, got %v", err)
	}
}

func TestUserRepo_GetByIdentifier(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()
	u := newTestUser(vo.MustParseId(userA), identA, "e", "Alice", "", "h", "s", true, fixedTime, fixedTime, nil)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByIdentifier(ctx, identA)
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if got.ID.String() != userA {
		t.Errorf("want %s, got %s", userA, got.ID)
	}
	_, err = repo.GetByIdentifier(ctx, "missing")
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for missing identifier, got %v", err)
	}
}

func TestUserRepo_ExistsByIdentifier(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()
	u := newTestUser(vo.MustParseId(userA), identA, "e", "Alice", "", "h", "s", true, fixedTime, fixedTime, nil)
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}
	exists, err := repo.ExistsByIdentifier(ctx, identA)
	if err != nil || !exists {
		t.Errorf("ExistsByIdentifier(ident-a) = %v, %v; want true", exists, err)
	}
	exists, err = repo.ExistsByIdentifier(ctx, "nope")
	if err != nil || exists {
		t.Errorf("ExistsByIdentifier(nope) = %v, %v; want false", exists, err)
	}
}

func TestUserRepo_ListIDs(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()
	for _, id := range []string{userA, userB} {
		u := newTestUser(vo.MustParseId(id), id[:32], "e", "U", "", "h", "s", true, fixedTime, fixedTime, nil)
		if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	ids, err := repo.ListIDs(ctx)
	if err != nil {
		t.Fatalf("ListIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 ids, got %d", len(ids))
	}
}

func TestUserRepo_GetOptions(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()
	val := "dark"
	opt := model.ReconstituteUserOption(vo.MustParseId(optID), "theme", &val, fixedTime, fixedTime)
	u := newTestUser(vo.MustParseId(userA), identA, "e", "Alice", "", "h", "s", true, fixedTime, fixedTime, []model.UserOption{opt})
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}
	opts, err := repo.GetOptions(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetOptions: %v", err)
	}
	if len(opts) != 1 || opts[0].Name != "theme" {
		t.Errorf("options mismatch: %+v", opts)
	}
}

func TestUserReadRepo_Views(t *testing.T) {
	repo, read, db := newRepos(t)
	ctx := context.Background()
	val := "light"
	opt := model.ReconstituteUserOption(vo.MustParseId(optID), "theme", &val, fixedTime, fixedTime)
	u := newTestUser(vo.MustParseId(userA), identA, "enc", "Alice", "https://av", "h", "s", true, fixedTime, fixedTime, []model.UserOption{opt})
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	uv, err := read.UserView(ctx, userA)
	if err != nil {
		t.Fatalf("UserView: %v", err)
	}
	if uv.ID != userA || uv.Email != "enc" || uv.Name != "Alice" || uv.Avatar != "https://av" {
		t.Errorf("UserView mismatch: %+v", uv)
	}

	_, err = read.UserView(ctx, vo.NewId().String())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for missing user view, got %v", err)
	}

	ov, err := read.OptionViews(ctx, userA)
	if err != nil {
		t.Fatalf("OptionViews: %v", err)
	}
	if len(ov) != 1 || ov[0].Name != "theme" || ov[0].Value == nil || *ov[0].Value != "light" {
		t.Errorf("OptionViews mismatch: %+v", ov)
	}
}

func TestUserReadRepo_CurrencyIDByCode(t *testing.T) {
	_, read, _ := newRepos(t)
	ctx := context.Background()
	// USD is seeded by the baseline migration (global, so resolves for anyone).
	id, err := read.CurrencyIDByCode(ctx, userA, "USD")
	if err != nil {
		t.Fatalf("CurrencyIDByCode(USD): %v", err)
	}
	if id != "dffc2a06-6f29-4704-8575-31709adee926" {
		t.Errorf("want seeded USD id, got %q", id)
	}
}

func TestUserReadRepo_CurrencyIDByCode_OwnCustomFirst(t *testing.T) {
	_, read, db := newRepos(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	owner := f.User(fixture.User{ID: "a3000000-0000-7000-8000-000000000001", Name: "Owner"})
	other := f.User(fixture.User{ID: "b3000000-0000-7000-8000-000000000002", Name: "Other"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: owner})

	id, err := read.CurrencyIDByCode(ctx, owner, "PTS")
	if err != nil || id != pts {
		t.Fatalf("own custom: got %q err %v", id, err)
	}
	if _, err := read.CurrencyIDByCode(ctx, other, "PTS"); err == nil {
		t.Fatal("foreign custom code must not resolve")
	}
}

func TestUserRepo_AlgorithmRoundTrip(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	u := newTestUser(
		vo.MustParseId(userA), identA, "enc-email", "Alice", "https://av/a",
		"hash", "salt-a", true, fixedTime, fixedTime, nil,
	)
	u.Algorithm = model.AlgorithmArgon2id
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByID(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Algorithm != model.AlgorithmArgon2id {
		t.Errorf("Algorithm = %q, want %q", got.Algorithm, model.AlgorithmArgon2id)
	}

	// A row inserted without an explicit algorithm gets the sha512 default.
	if _, err := db.Raw.Exec(db.Rebind(
		`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE)`),
		userB, "fedcba9876543210fedcba9876543210", "e2", "Bob", "", "h2", "s2", fixedTime, fixedTime); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	legacy, err := repo.GetByID(ctx, vo.MustParseId(userB))
	if err != nil {
		t.Fatalf("GetByID legacy: %v", err)
	}
	if legacy.Algorithm != model.AlgorithmSHA512 {
		t.Errorf("legacy Algorithm = %q, want %q", legacy.Algorithm, model.AlgorithmSHA512)
	}
}
