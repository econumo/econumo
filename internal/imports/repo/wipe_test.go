package repo_test

import (
	"context"
	"database/sql"
	"testing"

	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

type appliedRefs struct{ category, payee, tag, rule string }

func setApplied(t *testing.T, db *dbtest.DB, linkID string, r appliedRefs) {
	t.Helper()
	if _, err := db.Raw.ExecContext(context.Background(),
		db.Rebind("UPDATE import_transaction_links SET applied_category_id = ?, applied_payee_id = ?, applied_tag_id = ?, applied_rule_id = ? WHERE id = ?"),
		r.category, r.payee, r.tag, r.rule, linkID); err != nil {
		t.Fatal(err)
	}
}

func readApplied(t *testing.T, db *dbtest.DB, linkID string) appliedRefs {
	t.Helper()
	var c, p, tg, r sql.NullString
	if err := db.Raw.QueryRowContext(context.Background(),
		db.Rebind("SELECT applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id FROM import_transaction_links WHERE id = ?"),
		linkID).Scan(&c, &p, &tg, &r); err != nil {
		t.Fatal(err)
	}
	return appliedRefs{c.String, p.String, tg.String, r.String}
}

type importUser struct {
	id     vo.Id
	source string
	refs   appliedRefs
}

func seedImportUser(f *fixture.Builder, email string) importUser {
	id := vo.NewId()
	f.User(fixture.User{ID: id.String(), Email: email})
	return importUser{
		id:     id,
		source: f.ImportSource(fixture.ImportSource{UserID: id.String()}),
		refs: appliedRefs{
			category: f.Category(fixture.Category{UserID: id.String()}),
			payee:    f.Payee(fixture.Payee{UserID: id.String()}),
			tag:      f.Tag(fixture.Tag{UserID: id.String()}),
			rule:     f.ImportRule(fixture.ImportRule{UserID: id.String()}),
		},
	}
}

func TestClearAppliedRefsToUser(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	repo := importsrepo.NewRepo(db.Engine, db.TX)
	gone := seedImportUser(f, "gone@example.test")
	other := seedImportUser(f, "other@example.test")

	link := func(source, ext string) string {
		return f.ImportTransactionLink(fixture.ImportTransactionLink{SourceID: source, ExternalAccountID: "acc", ExternalTransactionID: ext})
	}
	pointsAtGone := link(other.source, "t1")
	setApplied(t, db, pointsAtGone, gone.refs)
	pointsAtOwn := link(other.source, "t2")
	setApplied(t, db, pointsAtOwn, other.refs)
	goneOwn := link(gone.source, "t3")
	setApplied(t, db, goneOwn, gone.refs)

	if err := repo.ClearAppliedRefsToUser(context.Background(), gone.id); err != nil {
		t.Fatalf("ClearAppliedRefsToUser: %v", err)
	}

	if got := readApplied(t, db, pointsAtGone); got != (appliedRefs{}) {
		t.Errorf("another user's link still points at the deleted user's entities: %+v", got)
	}
	if got := readApplied(t, db, pointsAtOwn); got != other.refs {
		t.Errorf("a link pointing at its own user's entities changed: %+v, want %+v", got, other.refs)
	}
	if got := readApplied(t, db, goneOwn); got != gone.refs {
		t.Errorf("the deleted user's own link changed: %+v, want %+v", got, gone.refs)
	}
}
