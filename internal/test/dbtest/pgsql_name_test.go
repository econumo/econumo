//go:build enginecompare

package dbtest

import "testing"

func TestPgSchemaNameKeepsLongSiblingsApart(t *testing.T) {
	base := "TestRevisionGuard_PostgreSQL/api/import_transaction_suggestion_"
	a, b := pgSchemaName(base+"list"), pgSchemaName(base+"fill")
	if a == b {
		t.Fatalf("both names map to %q", a)
	}
	for _, n := range []string{a, b} {
		if len(n) > 63 {
			t.Fatalf("%q is %d chars, over the 63-char limit", n, len(n))
		}
	}
	if got := pgSchemaName("TestShort"); got != "test_testshort" {
		t.Fatalf("short name changed: %q", got)
	}
}
