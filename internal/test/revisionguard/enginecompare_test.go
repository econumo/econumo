//go:build enginecompare

package revisionguard

import (
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// NewPostgres skips each scenario when DATABASE_TEST_PGSQL_URL is unset.
func TestRevisionGuard_PostgreSQL(t *testing.T) {
	Check(t, dbtest.NewPostgres)
}
