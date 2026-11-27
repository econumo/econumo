package account_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestUnwindGrantsOf(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{Name: "Wiped", Email: "wipe-u@example.test"})
	p := f.User(fixture.User{Name: "Partner", Email: "wipe-p@example.test"})
	tu := f.User(fixture.User{Name: "Third", Email: "wipe-t@example.test"})

	acctP := f.Account(fixture.Account{UserID: p, Name: "P own"})
	folderP := f.Folder(fixture.Folder{UserID: p, Name: "P main"})
	f.AccountInFolder(folderP, acctP)
	f.AccountOption(acctP, p, 1)

	folderU := f.Folder(fixture.Folder{UserID: u, Name: "U main"})
	f.AccountAccess(acctP, u, int(model.RoleUser))
	f.AccountInFolder(folderU, acctP)
	f.AccountOption(acctP, u, 1)

	acctT := f.Account(fixture.Account{UserID: tu, Name: "T own"})
	f.AccountAccessPending(acctT, u, int(model.RoleUser))

	acctU := f.Account(fixture.Account{UserID: u, Name: "U own"})
	f.AccountInFolder(folderU, acctU)
	f.AccountOption(acctU, u, 2)
	f.AccountAccess(acctU, p, int(model.RoleAdmin))
	f.AccountInFolder(folderP, acctU)
	f.AccountOption(acctU, p, 2)

	svc := newAccessSvc(t, db)
	ctx := context.Background()
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error {
		return svc.UnwindGrantsOf(ctx, mustParseID(t, u))
	}); err != nil {
		t.Fatalf("UnwindGrantsOf: %v", err)
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.Raw.QueryRow(db.Rebind(query), args...).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", query, err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM accounts_access WHERE user_id = ? OR account_id IN (SELECT id FROM accounts WHERE user_id = ?)`, u, u); n != 0 {
		t.Errorf("accounts_access rows naming U or one of U's accounts = %d, want 0", n)
	}
	if n := count(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`, acctU, p); n != 0 {
		t.Errorf("P's accounts_options row on U's account = %d, want 0", n)
	}
	if n := count(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`, acctP, u); n != 0 {
		t.Errorf("U's accounts_options row on P's account = %d, want 0", n)
	}
	if n := count(`SELECT COUNT(*) FROM accounts_folders WHERE folder_id = ? AND account_id = ?`, folderU, acctP); n != 0 {
		t.Errorf("U's folder still holds P's account")
	}

	var held []string
	rows, err := db.Raw.Query(db.Rebind(`SELECT account_id FROM accounts_folders WHERE folder_id = ?`), folderP)
	if err != nil {
		t.Fatalf("list P's folder: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		held = append(held, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(held) != 1 || held[0] != acctP {
		t.Errorf("P's folder holds %v, want only P's own account %s", held, acctP)
	}
	if n := count(`SELECT COUNT(*) FROM accounts_options WHERE account_id = ? AND user_id = ?`, acctP, p); n != 1 {
		t.Errorf("P's own accounts_options row = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM accounts WHERE user_id = ?`, p); n != 1 {
		t.Errorf("P's accounts = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM folders WHERE user_id = ?`, p); n != 1 {
		t.Errorf("P's folders = %d, want 1", n)
	}
}
