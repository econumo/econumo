package revisionguard

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestRevisionGuard_SQLite(t *testing.T) {
	Check(t, dbtest.NewSQLite)
}
