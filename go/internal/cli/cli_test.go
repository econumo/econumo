package cli

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// TestParseVerbosity covers flag accumulation, quiet, level mapping, and that
// the recognized flags are stripped while everything else is preserved in order.
func TestParseVerbosity(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantV     int
		wantQuiet bool
		wantRest  []string
		wantLevel slog.Level
	}{
		{"none", []string{"app:add-currency", "EUR"}, 0, false, []string{"app:add-currency", "EUR"}, slog.LevelWarn},
		{"single -v", []string{"app:x", "-v"}, 1, false, []string{"app:x"}, slog.LevelInfo},
		{"-vv", []string{"-vv", "app:x"}, 2, false, []string{"app:x"}, slog.LevelDebug},
		{"-vvv", []string{"app:x", "-vvv"}, 3, false, []string{"app:x"}, slog.LevelDebug},
		{"additive -v -v", []string{"app:x", "-v", "-v"}, 2, false, []string{"app:x"}, slog.LevelDebug},
		{"--verbose", []string{"--verbose", "app:x"}, 1, false, []string{"app:x"}, slog.LevelInfo},
		{"quiet wins", []string{"app:x", "-q", "-vvv"}, 3, true, []string{"app:x"}, slog.LevelError + 4},
		{"flags interleaved", []string{"app:update-currency-rates", "-vv", "2025-04-01"}, 2, false, []string{"app:update-currency-rates", "2025-04-01"}, slog.LevelDebug},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, quiet, rest := parseVerbosity(tc.args)
			if v != tc.wantV || quiet != tc.wantQuiet {
				t.Errorf("verbosity=%d quiet=%v, want %d/%v", v, quiet, tc.wantV, tc.wantQuiet)
			}
			if !reflect.DeepEqual(rest, tc.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tc.wantRest)
			}
			if got := levelFor(v, quiet); got != tc.wantLevel {
				t.Errorf("level = %v, want %v", got, tc.wantLevel)
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
