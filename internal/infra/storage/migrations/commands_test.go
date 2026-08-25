package migrations

import "testing"

func TestRegisterCommand_RejectsNonMigrationPrefixAndDuplicates(t *testing.T) {
	saved := registry
	t.Cleanup(func() { registry = saved })
	registry = nil
	RegisterCommand("99990101000000", "migration:demo")
	mustPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		f()
	}
	mustPanic(func() { RegisterCommand("99990101000001", "data:demo") })
	mustPanic(func() { RegisterCommand("99990101000000", "migration:other") })
}

func TestLoad_MergesCommandStepsInVersionOrder(t *testing.T) {
	saved := registry
	t.Cleanup(func() { registry = saved })
	registry = nil
	// A version after every real embedded migration, so the merged step lands
	// at the end of the list rather than coincidentally at index 0.
	RegisterCommand("29990101000000", "migration:demo")
	files := SQLite()
	found := -1
	for i, f := range files {
		if f.Command == "migration:demo" {
			found = i
		}
	}
	if found < 0 {
		t.Fatal("command step missing from SQLite() list")
	}
	if found == 0 || files[found-1].Version > files[found].Version {
		t.Fatalf("command step not in version order at %d", found)
	}
}
