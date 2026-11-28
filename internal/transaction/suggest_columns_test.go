package transaction_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

type columnsCompleter struct {
	calls  int
	system string
	user   string
	schema *ai.Schema
	reply  string
	err    error
}

func (c *columnsCompleter) Complete(_ context.Context, _, system, user string, schema *ai.Schema) (string, error) {
	c.calls++
	c.system, c.user, c.schema = system, user, schema
	return c.reply, c.err
}

type columnsFacts struct{ info apptransaction.AIFactsInfo }

func (f columnsFacts) AIFacts(context.Context, vo.Id) (apptransaction.AIFactsInfo, error) {
	return f.info, nil
}

type columnsLimiter struct{ deny bool }

func (l columnsLimiter) Allow(context.Context, string, string) error {
	if l.deny {
		return errs.NewTooManyRequests("Too many attempts. Try again later.")
	}
	return nil
}
func (columnsLimiter) Reserve(string, string) error { return nil }

var columnsUser = vo.MustParseId("11111111-1111-1111-1111-111111111111")

func columnsService(c apptransaction.Completer, facts apptransaction.AIFactsInfo, limiter apptransaction.AttemptLimiter) *apptransaction.Service {
	svc := apptransaction.NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if c != nil {
		svc.SetColumnAdvisor(c, columnsFacts{info: facts}, limiter)
	}
	return svc
}

func germanColumns() model.SuggestImportColumnsRequest {
	return model.SuggestImportColumnsRequest{Columns: []model.ImportColumnFact{
		{Name: "Buchungstag", Type: "date"},
		{Name: "Betrag", Type: "number", Negatives: true},
		{Name: "Verwendungszweck", Type: "text"},
	}}
}

// sure turns a role -> header reply into the reply shape with every pick sure;
// a null or non-string value is passed through as the whole pick.
func sure(roles map[string]any) string {
	out := make(map[string]any, len(roles))
	for role, v := range roles {
		if header, ok := v.(string); ok {
			out[role] = map[string]any{"header": header, "sure": true}
		} else {
			out[role] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestSuggestImportColumnsSendsOnlyTheAllowlist(t *testing.T) {
	c := &columnsCompleter{reply: `{}`}
	svc := columnsService(c, apptransaction.AIFactsInfo{Language: "de"}, columnsLimiter{})
	if _, err := svc.SuggestImportColumns(context.Background(), columnsUser, germanColumns()); err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 {
		t.Fatalf("calls = %d, want 1", c.calls)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(c.user), &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if got, want := sortedKeys(payload), []string{"columns", "language"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("payload keys = %v, want %v", got, want)
	}
	var columns []map[string]json.RawMessage
	if err := json.Unmarshal(payload["columns"], &columns); err != nil {
		t.Fatal(err)
	}
	if len(columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(columns))
	}
	for i, col := range columns {
		if got, want := sortedKeys(col), []string{"name", "negatives", "type"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("column %d keys = %v, want %v", i, got, want)
		}
	}
	if string(payload["language"]) != `"de"` {
		t.Fatalf("language = %s", payload["language"])
	}
	if c.schema == nil || c.schema.Name != "import_columns" {
		t.Fatalf("schema = %+v", c.schema)
	}
	if err := c.schema.Check(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(c.system) == "" {
		t.Fatal("empty system prompt")
	}
}

func TestSuggestImportColumnsDefaultsLanguage(t *testing.T) {
	c := &columnsCompleter{reply: `{}`}
	svc := columnsService(c, apptransaction.AIFactsInfo{}, columnsLimiter{})
	if _, err := svc.SuggestImportColumns(context.Background(), columnsUser, germanColumns()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.user, `"language":"en"`) {
		t.Fatalf("payload = %s", c.user)
	}
}

func TestSuggestImportColumnsKeepsOnlyKnownHeaders(t *testing.T) {
	c := &columnsCompleter{reply: "```json\n" + sure(map[string]any{"date": "Buchungstag", "amount": "Betrag", "description": "Nope"}) + "\n```"}
	svc := columnsService(c, apptransaction.AIFactsInfo{Language: "de"}, columnsLimiter{})
	res, err := svc.SuggestImportColumns(context.Background(), columnsUser, germanColumns())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"date": "Buchungstag", "amount": "Betrag"}
	if !reflect.DeepEqual(res.Roles, want) {
		t.Fatalf("roles = %v, want %v", res.Roles, want)
	}
}

func TestSuggestImportColumnsDropsUnknownRolesAndDuplicates(t *testing.T) {
	c := &columnsCompleter{reply: sure(map[string]any{"payee": "Verwendungszweck", "description": "Verwendungszweck", "amountOut": "Betrag", "amount": "Betrag", "memo": "Buchungstag", "date": nil, "tag": 7})}
	svc := columnsService(c, apptransaction.AIFactsInfo{}, columnsLimiter{})
	res, err := svc.SuggestImportColumns(context.Background(), columnsUser, germanColumns())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"amount": "Betrag", "description": "Verwendungszweck"}
	if !reflect.DeepEqual(res.Roles, want) {
		t.Fatalf("roles = %v, want %v", res.Roles, want)
	}
}

func TestSuggestImportColumnsRejectsRolesTheColumnTypeCannotHold(t *testing.T) {
	c := &columnsCompleter{reply: sure(map[string]any{"date": "Betrag", "amount": "Verwendungszweck", "amountIn": "Leer", "description": "Buchungstag", "payee": "Verwendungszweck", "category": "Saldo", "tag": "Leer"})}
	svc := columnsService(c, apptransaction.AIFactsInfo{Language: "de"}, columnsLimiter{})
	req := germanColumns()
	req.Columns = append(req.Columns,
		model.ImportColumnFact{Name: "Leer", Type: "empty"},
		model.ImportColumnFact{Name: "Saldo", Type: "number"},
	)
	res, err := svc.SuggestImportColumns(context.Background(), columnsUser, req)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"payee": "Verwendungszweck", "category": "Saldo"}
	if !reflect.DeepEqual(res.Roles, want) {
		t.Fatalf("roles = %v, want %v", res.Roles, want)
	}
}

func TestSuggestImportColumnsNullNeverNamesABlankHeader(t *testing.T) {
	c := &columnsCompleter{reply: `{"date":{"header":"Buchungstag","sure":true},"amount":{"header":"Betrag","sure":true},"description":{"header":null,"sure":true},"payee":{"header":"","sure":true}}`}
	svc := columnsService(c, apptransaction.AIFactsInfo{}, columnsLimiter{})
	req := germanColumns()
	req.Columns = append(req.Columns, model.ImportColumnFact{Name: "", Type: "text"})
	res, err := svc.SuggestImportColumns(context.Background(), columnsUser, req)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"date": "Buchungstag", "amount": "Betrag"}
	if !reflect.DeepEqual(res.Roles, want) {
		t.Fatalf("roles = %v, want %v", res.Roles, want)
	}
}

func TestSuggestImportColumnsDropsUnsurePicks(t *testing.T) {
	c := &columnsCompleter{reply: `{"date":{"header":"Buchungstag","sure":true},"amount":{"header":"Betrag","sure":false},"description":{"header":"Verwendungszweck"},"payee":"Verwendungszweck"}`}
	svc := columnsService(c, apptransaction.AIFactsInfo{}, columnsLimiter{})
	res, err := svc.SuggestImportColumns(context.Background(), columnsUser, germanColumns())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"date": "Buchungstag"}
	if !reflect.DeepEqual(res.Roles, want) {
		t.Fatalf("roles = %v, want %v", res.Roles, want)
	}
}

func TestSuggestImportColumnsEmptyWhen(t *testing.T) {
	cases := map[string]struct {
		completer *columnsCompleter
		facts     apptransaction.AIFactsInfo
		limiter   columnsLimiter
		wantCalls int
	}{
		"no_completer":    {},
		"ai_off":          {completer: &columnsCompleter{reply: `{"date":{"header":"Buchungstag","sure":true}}`}, facts: apptransaction.AIFactsInfo{AIOff: true}},
		"readonly":        {completer: &columnsCompleter{reply: `{"date":{"header":"Buchungstag","sure":true}}`}, facts: apptransaction.AIFactsInfo{Readonly: true}},
		"completer_error": {completer: &columnsCompleter{err: errors.New("boom")}, wantCalls: 1},
		"bad_json":        {completer: &columnsCompleter{reply: "the first one"}, wantCalls: 1},
		"rate_limited":    {completer: &columnsCompleter{reply: `{"date":{"header":"Buchungstag","sure":true}}`}, limiter: columnsLimiter{deny: true}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var c apptransaction.Completer
			if tc.completer != nil {
				c = tc.completer
			}
			svc := columnsService(c, tc.facts, tc.limiter)
			res, err := svc.SuggestImportColumns(context.Background(), columnsUser, germanColumns())
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if res == nil || res.Roles == nil || len(res.Roles) != 0 {
				t.Fatalf("roles = %+v, want empty", res)
			}
			b, _ := json.Marshal(res)
			if string(b) != `{"roles":{}}` {
				t.Fatalf("wire = %s", b)
			}
			if tc.completer != nil && tc.completer.calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", tc.completer.calls, tc.wantCalls)
			}
		})
	}
}

func TestSuggestImportColumnsValidate(t *testing.T) {
	many := make([]model.ImportColumnFact, 61)
	for i := range many {
		many[i] = model.ImportColumnFact{Name: "c", Type: "text"}
	}
	cases := map[string][]model.ImportColumnFact{
		"none":       nil,
		"61_columns": many,
		"empty_name": {{Name: "  ", Type: "text"}},
		"long_name":  {{Name: strings.Repeat("a", 101), Type: "text"}},
		"bad_type":   {{Name: "Amount", Type: "money"}},
	}
	for name, cols := range cases {
		t.Run(name, func(t *testing.T) {
			err := model.SuggestImportColumnsRequest{Columns: cols}.Validate()
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a validation error", err)
			}
		})
	}
	ok := model.SuggestImportColumnsRequest{Columns: []model.ImportColumnFact{
		{Name: strings.Repeat("a", 100), Type: "date"},
		{Name: "b", Type: "number", Negatives: true},
		{Name: "c", Type: "text"},
		{Name: "d", Type: "empty"},
	}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid request refused: %v", err)
	}
}
