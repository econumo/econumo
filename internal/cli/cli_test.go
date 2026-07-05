package cli

import (
	"strings"
	"testing"
)

// Verbosity/level resolution moved to internal/logging; see logging_test.go.

// TestCommandRegistry checks the registry is well-formed: unique names, the
// resource:action naming scheme, and a summary on every command.
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
		if !isResourceAction(c.name) {
			t.Errorf("command %q is not a valid resource:action name", c.name)
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary", c.name)
		}
		if c.run == nil {
			t.Errorf("command %q has no run func", c.name)
		}
	}

	// The full set of management commands.
	want := []string{
		"user:create", "user:change-email", "user:change-password",
		"user:activate", "user:deactivate",
		"currency:update-rates", "currency:add",
		"jwt:generate", "data:remove-salt",
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("expected command %q to be registered", name)
		}
	}
}

// isResourceAction reports whether name is a well-formed resource:action command
// name: exactly one ':' separating two non-empty lowercase-kebab segments (lowercase
// letters, digits, and internal hyphens).
func isResourceAction(name string) bool {
	parts := strings.Split(name, ":")
	if len(parts) != 2 {
		return false
	}
	return isKebabSegment(parts[0]) && isKebabSegment(parts[1])
}

// isKebabSegment reports whether s is non-empty, lowercase-kebab, and neither
// starts nor ends with a hyphen.
func isKebabSegment(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// TestRunUsagePaths covers the no-container dispatch paths: no args and unknown
// command return exit 2; help returns 0. (A known command would build the
// container/DB, which these cases never reach.)
func TestRunUsagePaths(t *testing.T) {
	if code := Run(nil); code != 2 {
		t.Errorf("Run(nil) = %d, want 2", code)
	}
	if code := Run([]string{"does-not-exist"}); code != 2 {
		t.Errorf("Run(unknown) = %d, want 2", code)
	}
	if code := Run([]string{"help"}); code != 0 {
		t.Errorf("Run(help) = %d, want 0", code)
	}
}

// TestFirstPositional covers the flag-skipping positional-arg extraction used
// by currency:update-rates' optional date argument.
func TestFirstPositional(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no args", []string{}, ""},
		{"only flags", []string{"-v", "-vv", "-q"}, ""},
		{"flags before positional", []string{"-v", "-vvv", "2024-01-02"}, "2024-01-02"},
		{"positional first", []string{"2024-01-02", "-v"}, "2024-01-02"},
		{"blank args skipped", []string{"  ", "2024-01-02"}, "2024-01-02"},
		{"trims whitespace", []string{"  2024-01-02  "}, "2024-01-02"},
		{"all blank", []string{"", "   "}, ""},
	}
	for _, c := range cases {
		if got := firstPositional(c.args); got != c.want {
			t.Errorf("%s: firstPositional(%q) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}

// TestIndexPanicsOnDuplicate guards the duplicate-name invariant.
func TestIndexPanicsOnDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("index did not panic on duplicate command names")
		}
	}()
	index([]command{{name: "x:y"}, {name: "x:y"}})
}
