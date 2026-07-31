package migrations_test

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestUsersTimezoneColumn(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	id := f.User(fixture.User{})
	var tz string
	if err := db.Raw.QueryRow(db.Rebind("SELECT timezone FROM users WHERE id = ?"), id).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	if tz != "" {
		t.Fatalf("default timezone = %q, want empty", tz)
	}
	if _, err := db.Raw.Exec(db.Rebind("UPDATE users SET timezone = ? WHERE id = ?"), "Europe/Amsterdam", id); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw.QueryRow(db.Rebind("SELECT timezone FROM users WHERE id = ?"), id).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	if tz != "Europe/Amsterdam" {
		t.Fatalf("timezone = %q", tz)
	}
}
