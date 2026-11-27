//go:build enginecompare

package wipeguard

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// NewPostgres skips when DATABASE_TEST_PGSQL_URL is unset.
func TestSchemaGuard_PostgreSQL(t *testing.T) {
	Check(t, dbtest.NewPostgres(t))
}

func TestUserColumns_PostgreSQL(t *testing.T) {
	assertUserColumns(t, dbtest.NewPostgres(t))
}
