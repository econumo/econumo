package cli

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// TestResolveVerbosity covers the Symfony-compatible aliases: short (-v/-vv/-vvv),
// long (--verbose, --verbose=N), quiet (-q/--quiet), the SHELL_VERBOSITY env
// baseline + flag override, priority (not additive) semantics, and flag stripping.
func TestResolveVerbosity(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		shellEnv  string
		wantLevel int
		wantQuiet bool
		wantRest  []string
		wantSlog  slog.Level
	}{
		{"none", []string{"app:add-currency", "EUR"}, "", 0, false, []string{"app:add-currency", "EUR"}, slog.LevelWarn},
		{"-v", []string{"app:x", "-v"}, "", 1, false, []string{"app:x"}, slog.LevelInfo},
		{"-vv", []string{"-vv", "app:x"}, "", 2, false, []string{"app:x"}, slog.LevelDebug},
		{"-vvv", []string{"app:x", "-vvv"}, "", 3, false, []string{"app:x"}, slog.LevelDebug},
		{"--verbose", []string{"--verbose", "app:x"}, "", 1, false, []string{"app:x"}, slog.LevelInfo},
		{"--verbose=2", []string{"app:x", "--verbose=2"}, "", 2, false, []string{"app:x"}, slog.LevelDebug},
		{"--verbose=3", []string{"app:x", "--verbose=3"}, "", 3, false, []string{"app:x"}, slog.LevelDebug},
		// Priority, NOT additive: two -v stay verbose (Symfony semantics).
		{"-v -v stays verbose", []string{"app:x", "-v", "-v"}, "", 1, false, []string{"app:x"}, slog.LevelInfo},
		// Highest flag wins regardless of order.
		{"mixed -v -vvv", []string{"-v", "app:x", "-vvv"}, "", 3, false, []string{"app:x"}, slog.LevelDebug},
		{"quiet beats -vvv", []string{"app:x", "-q", "-vvv"}, "", 0, true, []string{"app:x"}, slog.LevelError + 4},
		{"--quiet", []string{"--quiet", "app:x"}, "", 0, true, []string{"app:x"}, slog.LevelError + 4},
		{"flags interleaved kept in order", []string{"app:update-currency-rates", "-vv", "2025-04-01"}, "", 2, false, []string{"app:update-currency-rates", "2025-04-01"}, slog.LevelDebug},
		// SHELL_VERBOSITY baseline applies when no flag is given...
		{"env baseline 2", []string{"app:x"}, "2", 2, false, []string{"app:x"}, slog.LevelDebug},
		{"env quiet -1", []string{"app:x"}, "-1", 0, true, []string{"app:x"}, slog.LevelError + 4},
		// ...and a flag overrides the env baseline.
		{"flag overrides env", []string{"app:x", "-vvv"}, "1", 3, false, []string{"app:x"}, slog.LevelDebug},
		{"-v overrides env quiet", []string{"app:x", "-v"}, "-1", 1, false, []string{"app:x"}, slog.LevelInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level, quiet, rest := resolveVerbosity(tc.args, tc.shellEnv)
			if level != tc.wantLevel || quiet != tc.wantQuiet {
				t.Errorf("level=%d quiet=%v, want %d/%v", level, quiet, tc.wantLevel, tc.wantQuiet)
			}
			if !reflect.DeepEqual(rest, tc.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tc.wantRest)
			}
			if got := levelFor(level, quiet); got != tc.wantSlog {
				t.Errorf("slog level = %v, want %v", got, tc.wantSlog)
			}
		})
	}
}

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
		"app:update-currency-rates", "app:add-currency", "app:restore-currency-fraction-digits",
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
