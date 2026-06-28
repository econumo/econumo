package logging

import (
	"log/slog"
	"reflect"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":    slog.LevelDebug,
		"DEBUG":    slog.LevelDebug,
		"info":     slog.LevelInfo,
		" Info ":   slog.LevelInfo,
		"warn":     slog.LevelWarn,
		"warning":  slog.LevelWarn,
		"error":    slog.LevelError,
		"err":      slog.LevelError,
		"":         slog.LevelInfo, // default
		"nonsense": slog.LevelInfo, // unknown -> default INFO
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestResolveVerbosity covers the Symfony-compatible aliases and flag stripping.
func TestResolveVerbosity(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		shellEnv  string
		wantLevel int
		wantQuiet bool
		wantRest  []string
	}{
		{"none", []string{"app:add-currency", "EUR"}, "", 0, false, []string{"app:add-currency", "EUR"}},
		{"-v", []string{"app:x", "-v"}, "", 1, false, []string{"app:x"}},
		{"-vv", []string{"-vv", "app:x"}, "", 2, false, []string{"app:x"}},
		{"-vvv", []string{"app:x", "-vvv"}, "", 3, false, []string{"app:x"}},
		{"--verbose", []string{"--verbose", "app:x"}, "", 1, false, []string{"app:x"}},
		{"--verbose=2", []string{"app:x", "--verbose=2"}, "", 2, false, []string{"app:x"}},
		// Priority, NOT additive: two -v stay verbose (Symfony semantics).
		{"-v -v stays verbose", []string{"app:x", "-v", "-v"}, "", 1, false, []string{"app:x"}},
		// Highest flag wins regardless of order.
		{"mixed -v -vvv", []string{"-v", "app:x", "-vvv"}, "", 3, false, []string{"app:x"}},
		{"quiet beats -vvv", []string{"app:x", "-q", "-vvv"}, "", 0, true, []string{"app:x"}},
		{"--quiet", []string{"--quiet", "app:x"}, "", 0, true, []string{"app:x"}},
		{"flags interleaved kept in order", []string{"app:update-currency-rates", "-vv", "2025-04-01"}, "", 2, false, []string{"app:update-currency-rates", "2025-04-01"}},
		// SHELL_VERBOSITY baseline applies when no flag is given...
		{"env baseline 2", []string{"app:x"}, "2", 2, false, []string{"app:x"}},
		{"env quiet -1", []string{"app:x"}, "-1", 0, true, []string{"app:x"}},
		// ...and a flag overrides the env baseline.
		{"flag overrides env", []string{"app:x", "-vvv"}, "1", 3, false, []string{"app:x"}},
		{"-v overrides env quiet", []string{"app:x", "-v"}, "-1", 1, false, []string{"app:x"}},
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
		})
	}
}

// TestEffectiveLevel verifies the INFO-default re-basing: no flag keeps the
// baseline, any -v drops to DEBUG, -vvv adds source, -q silences.
func TestEffectiveLevel(t *testing.T) {
	cases := []struct {
		name          string
		baseline      slog.Level
		level         int
		quiet         bool
		wantLevel     slog.Level
		wantAddSource bool
	}{
		{"baseline info, no flag", slog.LevelInfo, 0, false, slog.LevelInfo, false},
		{"baseline warn, no flag", slog.LevelWarn, 0, false, slog.LevelWarn, false},
		{"-v -> debug", slog.LevelInfo, 1, false, slog.LevelDebug, false},
		{"-vv -> debug", slog.LevelInfo, 2, false, slog.LevelDebug, false},
		{"-vvv -> debug + source", slog.LevelInfo, 3, false, slog.LevelDebug, true},
		{"quiet silences", slog.LevelInfo, 0, true, slog.LevelError + 4, false},
		{"quiet beats verbose", slog.LevelInfo, 3, true, slog.LevelError + 4, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lvl, addSource := effectiveLevel(tc.baseline, tc.level, tc.quiet)
			if lvl != tc.wantLevel || addSource != tc.wantAddSource {
				t.Errorf("got (%v, %v), want (%v, %v)", lvl, addSource, tc.wantLevel, tc.wantAddSource)
			}
		})
	}
}

// TestSetup confirms it strips flags from the returned args and installs a
// handler at the resolved level.
func TestSetup(t *testing.T) {
	t.Setenv("SHELL_VERBOSITY", "")
	rest := Setup("warn", []string{"app:x", "-vvv", "pos"})
	if !reflect.DeepEqual(rest, []string{"app:x", "pos"}) {
		t.Fatalf("rest = %v, want [app:x pos]", rest)
	}
	// -vvv forces DEBUG regardless of the "warn" baseline.
	if !slog.Default().Enabled(nil, slog.LevelDebug) {
		t.Error("expected DEBUG enabled after Setup with -vvv")
	}

	// Baseline alone (no flags) is honored.
	_ = Setup("warn", []string{"app:x"})
	if slog.Default().Enabled(nil, slog.LevelInfo) {
		t.Error("expected INFO disabled with warn baseline and no flags")
	}
	if !slog.Default().Enabled(nil, slog.LevelWarn) {
		t.Error("expected WARN enabled with warn baseline")
	}
}
