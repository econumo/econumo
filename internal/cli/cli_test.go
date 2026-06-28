package cli

import (
	"strings"
	"testing"
)

// Verbosity/level resolution moved to internal/logging; see logging_test.go.

// TestCommandRegistry checks the registry is well-formed: unique names, the
// Symfony "app:" prefix, and a summary on every command.
func TestCommandRegistry(t *testing.T) {
	cs := commandList()
	if len(cs) == 0 {
		t.Fatal("no commands registered")
	}
	seen := map[string]bool{}
	for _, c := range cs {
		if seen[c.name] {
			t.Errorf("duplicate command name %q", c.name)
		}
		seen[c.name] = true
		if !strings.HasPrefix(c.name, "app:") {
			t.Errorf("command %q does not use the app: prefix", c.name)
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary", c.name)
		}
		if c.run == nil {
			t.Errorf("command %q has no run func", c.name)
		}
	}

	// The full set ported in this change.
	want := []string{
		"app:create-user", "app:change-user-email", "app:change-user-password",
		"app:activate-user", "app:deactivate-users",
		"app:update-currency-rates", "app:add-currency",
		"app:generate-jwt-keypair",
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("expected command %q to be registered", name)
		}
	}
}

// TestRunUsagePaths covers the no-container dispatch paths: no args and unknown
// command return exit 2; help returns 0. (A known command would build the
// container/DB, which these cases never reach.)
func TestRunUsagePaths(t *testing.T) {
	if code := Run(nil); code != 2 {
		t.Errorf("Run(nil) = %d, want 2", code)
	}
	if code := Run([]string{"app:does-not-exist"}); code != 2 {
		t.Errorf("Run(unknown) = %d, want 2", code)
	}
	if code := Run([]string{"help"}); code != 0 {
		t.Errorf("Run(help) = %d, want 0", code)
	}
}

// TestIndexPanicsOnDuplicate guards the duplicate-name invariant.
func TestIndexPanicsOnDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("index did not panic on duplicate command names")
		}
	}()
	index([]command{{name: "app:x"}, {name: "app:x"}})
}
