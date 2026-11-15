//go:build aieval

// Package aieval is the support code of the AI eval suite: the real provider
// from ECONUMO_AI_DSN and the per-area scoring. The suites live next to the
// prompts they measure (internal/imports, internal/onboarding) under the
// same build tag, so they drive the production prompt builders, schemas and
// parsers; `make ai-eval` runs them.
package aieval

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/ai"
)

// PassThreshold is the share of an area's cases (per language) that must
// pass. Models drift between runs, so up to 10% may fail before the area does.
const PassThreshold = 0.90

// EvalQuery is the model and options the suite measures. The DSN usually names
// only the provider, key and host (`anthropic://<key>@api.anthropic.com`), so
// the measured model lives in the code, next to the cases it was tuned on; a
// DSN that carries its own query (a local experiment) is used as given.
const EvalQuery = "model=claude-haiku-5-5&json=1"

// Completer is the configured provider. Each call that fails is retried once,
// so a transient provider error does not count as a wrong answer.
type Completer struct {
	client   *ai.Client
	Calls    atomic.Int64
	CharsIn  atomic.Int64
	CharsOut atomic.Int64
}

// Provider returns the provider ECONUMO_AI_DSN names, or skips the test when
// it is unset. Neither the DSN nor the key is ever logged.
func Provider(t *testing.T) *Completer {
	t.Helper()
	dsn := os.Getenv("ECONUMO_AI_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("ECONUMO_AI_DSN is not set: the AI eval suite needs a real provider")
	}
	if !strings.Contains(dsn, "?") {
		dsn = strings.TrimRight(dsn, "/") + "?" + EvalQuery
	}
	dialect, endpoint, key, model, jsonSchema, err := config.ParseAIDSN(dsn)
	if err != nil {
		t.Fatalf("ECONUMO_AI_DSN is invalid: %v", err)
	}
	if dialect == config.AIDialectEconumo {
		t.Skip("the AI eval suite needs an openai:// or anthropic:// DSN (econumo:// calls are signed per user)")
	}
	t.Logf("provider: %s, model %s, json schema %v", dialect, model, jsonSchema)
	return &Completer{client: ai.New(ai.Config{Dialect: dialect, Endpoint: endpoint, APIKey: key, Model: model, JSONSchema: jsonSchema})}
}

func (c *Completer) Complete(ctx context.Context, userID, system, user string, schema *ai.Schema) (string, error) {
	var reply string
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		c.Calls.Add(1)
		c.CharsIn.Add(int64(len(system) + len(user)))
		if reply, err = c.client.Complete(ctx, userID, system, user, schema); err == nil {
			c.CharsOut.Add(int64(len(reply)))
			return reply, nil
		}
	}
	return "", err
}

// Spend logs the calls and the bytes sent and received; a byte count is a
// rough upper bound of the tokens (Cyrillic takes two bytes a letter).
func (c *Completer) Spend(t *testing.T) {
	t.Logf("spend: %d calls, %d bytes in, %d bytes out", c.Calls.Load(), c.CharsIn.Load(), c.CharsOut.Load())
}

// Area scores the cases of one area in one language.
type Area struct {
	Name   string
	mu     sync.Mutex
	total  int
	failed []string
}

func NewArea(name string) *Area { return &Area{Name: name} }

// Check records one case; a failure keeps "input: got X, want Y".
func (a *Area) Check(pass bool, input, got, want string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.total++
	if !pass {
		a.failed = append(a.failed, fmt.Sprintf("%s: got %s, want %s", input, got, want))
	}
}

func (a *Area) rate() float64 {
	if a.total == 0 {
		return 0
	}
	return float64(a.total-len(a.failed)) / float64(a.total)
}

// Finish reports the areas as a table, their failed cases below it, and fails
// the test when an area passes fewer than PassThreshold of its cases. With
// AIEVAL_TABLE / AIEVAL_FAILURES set (the workflow and `make ai-eval`), the
// rows and failures are also appended to those files as Markdown.
func Finish(t *testing.T, areas ...*Area) {
	t.Helper()
	var rows, failures strings.Builder
	for _, a := range areas {
		status := "pass"
		if a.rate() < PassThreshold {
			status = "FAIL"
		}
		fmt.Fprintf(&rows, "| %s | %d/%d | %.0f%% | %.0f%% | %s |\n", a.Name, a.total-len(a.failed), a.total, a.rate()*100, PassThreshold*100, status)
		for _, f := range a.failed {
			fmt.Fprintf(&failures, "- `%s` %s\n", a.Name, f)
		}
	}
	t.Logf("\n| Area | Passed | Rate | Threshold | Status |\n|---|---|---|---|---|\n%s", rows.String())
	if failures.Len() > 0 {
		t.Logf("failed cases:\n%s", failures.String())
	}
	appendTo(t, os.Getenv("AIEVAL_TABLE"), rows.String())
	appendTo(t, os.Getenv("AIEVAL_FAILURES"), failures.String())
	for _, a := range areas {
		if a.total == 0 {
			t.Errorf("%s: no cases ran", a.Name)
		} else if a.rate() < PassThreshold {
			t.Errorf("%s: %.0f%% of cases passed, below the %.0f%% threshold", a.Name, a.rate()*100, PassThreshold*100)
		}
	}
}

func appendTo(t *testing.T, path, text string) {
	if path == "" || text == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Errorf("aieval: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Errorf("aieval: %v", err)
	}
}

// Languages are the UI languages every case runs in, with a timezone that
// fits the cases' names.
var Languages = []struct{ Code, Timezone string }{
	{"en", "America/Toronto"},
	{"ru", "Europe/Moscow"},
}

// OneOf reports whether got is one of want; "" in want accepts no answer.
func OneOf(got string, want []string) bool {
	for _, w := range want {
		if strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(w)) {
			return true
		}
	}
	return false
}

// Show renders an answer for a failure line, "none" for no answer.
func Show(s string) string {
	if s == "" {
		return "none"
	}
	return fmt.Sprintf("%q", s)
}

// ShowSet renders an acceptable set for a failure line.
func ShowSet(want []string) string {
	out := make([]string, len(want))
	for i, w := range want {
		out[i] = Show(w)
	}
	return strings.Join(out, "|")
}
