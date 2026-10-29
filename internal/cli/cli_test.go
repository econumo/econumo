package cli

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
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
		"currency:show-rate-gaps", "currency:backfill-rates",
		"token:purge",
		"data:remove-salt",
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

// TestLogCommandStartOmitsSecrets is the regression guard for the leak where the
// command-start DEBUG line logged the full argument vector — for user:create and
// user:change-password that included the plaintext password and email.
func TestLogCommandStartOmitsSecrets(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	const email, password = "secret@example.test", "SuperSecret123"
	logCommandStart("user:create", []string{"Test Name", email, password})

	out := buf.String()
	if strings.Contains(out, email) {
		t.Errorf("DEBUG line leaked the email:\n%s", out)
	}
	if strings.Contains(out, password) {
		t.Errorf("DEBUG line leaked the password:\n%s", out)
	}
	if !strings.Contains(out, "user:create") {
		t.Errorf("DEBUG line dropped the command name, its only useful signal:\n%s", out)
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

func TestParseRateGapArgs(t *testing.T) {
	const usage = "currency:show-rate-gaps [start] [end] [--daily]"
	a, err := parseRateGapArgs([]string{"2026-01", "2026-03-15", "--daily"}, usage)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.from.Format("2006-01-02") != "2026-01-01" || a.to.Format("2006-01-02") != "2026-03-15" || a.period != model.RatePeriodDay {
		t.Errorf("got from=%s to=%s period=%d", a.from, a.to, a.period)
	}

	a, err = parseRateGapArgs(nil, usage)
	if err != nil || !a.from.IsZero() || !a.to.IsZero() || a.period != model.RatePeriodMonth {
		t.Errorf("no args: %+v err=%v, want zero dates, monthly", a, err)
	}

	for _, bad := range [][]string{
		{"--dialy"},
		{"-daily"},
		{"2026-01", "2026-02", "2026-03"},
		{"2026/01/01"},
	} {
		if _, err := parseRateGapArgs(bad, usage); err == nil {
			t.Errorf("parseRateGapArgs(%q) accepted", bad)
		}
	}
}

func TestPrintRateGaps(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	printRateGaps(&buf, model.RateGapReport{
		Base: "USD", From: jan, To: mar, Currencies: []string{"EUR", "GBP"},
		Gaps: []model.RateGap{{Start: mar, Missing: []string{"GBP"}}},
	})
	want := "Rates against USD for EUR, GBP, 2026-01 to 2026-03\n2026-03  missing GBP\n1 month with gaps\n"
	if buf.String() != want {
		t.Errorf("output =\n%s\nwant\n%s", buf.String(), want)
	}

	buf.Reset()
	printRateBackfill(&buf, model.RateBackfill{Date: mar, Requested: []string{"EUR", "GBP"}, Stored: 1, Unavailable: []string{"GBP"}})
	if got := buf.String(); got != "2026-03-01  stored 1 rate(s); no rate returned for GBP\n" {
		t.Errorf("backfill line = %q", got)
	}
}
