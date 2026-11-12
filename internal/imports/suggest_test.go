package imports

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"reflect"
)

func sLink(payee, desc string, posted time.Time) model.ImportTransactionLink {
	return model.ImportTransactionLink{ID: vo.NewId(), ExternalPayee: payee, ExternalDescription: desc, ExternalPostedAt: posted}
}

func sRow(link model.ImportTransactionLink) suggestRow {
	r := suggestRow{Link: link, Type: model.TransactionTypeExpense, Amount: "5.00", Currency: "USD", Account: "Card"}
	r.Missing.Category, r.Missing.Payee = true, true
	return r
}

func sVocab() (suggestVocabulary, model.ImportNamed, model.ImportNamed, model.ImportNamed, model.ImportNamed) {
	coffee := model.ImportNamed{ID: vo.NewId().String(), Name: "Coffee"}
	dining := model.ImportNamed{ID: vo.NewId().String(), Name: "Dining"}
	salary := model.ImportNamed{ID: vo.NewId().String(), Name: "Salary", Income: true}
	bb := model.ImportNamed{ID: vo.NewId().String(), Name: "Blue Bottle"}
	return newSuggestVocabulary([]model.ImportNamed{coffee, dining, salary}, []model.ImportNamed{bb}), coffee, dining, salary, bb
}

func TestMatchTextIn(t *testing.T) {
	l := sLink("SQ *BLUE BOTTLE COFFEE #0423", "memo", time.Time{})
	f, n, ok := matchTextIn(l, "blue bottle  coffee")
	require.True(t, ok)
	require.Equal(t, model.ImportRuleMatchFieldExternalPayee, f)
	require.Equal(t, "blue bottle coffee", n)
	_, _, ok = matchTextIn(sLink("STARBUCKS", "", time.Time{}), "Peets")
	require.False(t, ok)
	f, _, ok = matchTextIn(sLink("  ", "Coffee at Peets", time.Time{}), "peets")
	require.True(t, ok)
	require.Equal(t, model.ImportRuleMatchFieldDescription, f)
	_, _, ok = matchTextIn(l, "   ")
	require.False(t, ok)
}

func TestValidateReplyDropsInventedIDs(t *testing.T) {
	v, _, _, _, bb := sVocab()
	rows := []suggestRow{sRow(sLink("BLUE BOTTLE", "", time.Time{}))}
	got := validateReply(rows, map[string]suggestReplyRow{"0": {MatchText: "BLUE BOTTLE", CategoryID: vo.NewId().String(), PayeeID: bb.ID}}, v)
	require.Len(t, got, 1)
	require.Nil(t, got[0].CategoryID)
	require.Equal(t, bb.ID, got[0].PayeeID.String())
}

func TestValidateReplyDropsWrongTypeCategory(t *testing.T) {
	v, _, _, salary, _ := sVocab()
	rows := []suggestRow{sRow(sLink("BLUE BOTTLE", "", time.Time{}))}
	got := validateReply(rows, map[string]suggestReplyRow{"0": {MatchText: "BLUE BOTTLE", CategoryID: salary.ID}}, v)
	require.Empty(t, got)
}

func TestValidateReplyDropsMatchTextNotInRow(t *testing.T) {
	v, coffee, _, _, _ := sVocab()
	rows := []suggestRow{sRow(sLink("BLUE BOTTLE", "", time.Time{}))}
	got := validateReply(rows, map[string]suggestReplyRow{"0": {MatchText: "PEETS", CategoryID: coffee.ID}}, v)
	require.Empty(t, got)
}

func TestValidateReplyDropsFieldAlreadySet(t *testing.T) {
	v, coffee, _, _, bb := sVocab()
	row := sRow(sLink("BLUE BOTTLE", "", time.Time{}))
	row.Missing.Payee = false
	got := validateReply([]suggestRow{row}, map[string]suggestReplyRow{"0": {MatchText: "BLUE", CategoryID: coffee.ID, PayeeID: bb.ID}}, v)
	require.Len(t, got, 1)
	require.Nil(t, got[0].PayeeID)
	require.NotNil(t, got[0].CategoryID)
}

func TestParseSuggestReplyRejectsGarbage(t *testing.T) {
	_, err := parseSuggestReply("sorry, no")
	require.Error(t, err)
	_, err = parseSuggestReply(`{"other":1}`)
	require.Error(t, err)
	m, err := parseSuggestReply(`{"rows":{"0":{"matchText":"x"}}}`)
	require.NoError(t, err)
	require.Equal(t, "x", m["0"].MatchText)
}

func validOf(link model.ImportTransactionLink, text string, cat *vo.Id) validSuggestion {
	return validSuggestion{Row: sRow(link), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: text, CategoryID: cat}
}

func TestGroupRowsMajorityWins(t *testing.T) {
	coffee, dining := vo.NewId(), vo.NewId()
	var valid []validSuggestion
	for _, c := range []vo.Id{coffee, coffee, dining} {
		c := c
		valid = append(valid, validOf(sLink("BLUE BOTTLE", "", time.Time{}), "BLUE BOTTLE", &c))
	}
	got := groupRows(valid)
	require.Len(t, got, 1)
	require.Equal(t, coffee, *got[0].CategoryID)
	require.Len(t, got[0].LinkIDs, 3)
	require.Equal(t, model.ImportSuggestionStatusOpen, got[0].Status)
}

func TestGroupRowsTieGoesToNewest(t *testing.T) {
	coffee, dining := vo.NewId(), vo.NewId()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := groupRows([]validSuggestion{
		validOf(sLink("BB", "", old), "bb", &coffee),
		validOf(sLink("BB", "", old.AddDate(0, 0, 5)), "BB", &dining),
	})
	require.Len(t, got, 1)
	require.Equal(t, dining, *got[0].CategoryID)
}

func TestMergeIntoOpenJoinsContainedMatchText(t *testing.T) {
	coffee, dining := vo.NewId(), vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", CategoryID: &coffee}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE COFFEE", CategoryID: &dining, LinkIDs: []vo.Id{vo.NewId(), vo.NewId()}}
	fresh, joins := mergeIntoOpen([]model.ImportSuggestion{g}, []model.ImportSuggestion{open})
	require.Empty(t, fresh)
	require.Equal(t, g.LinkIDs, joins[open.ID])
}

func TestMergeIntoOpenKeepsDifferentField(t *testing.T) {
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldDescription, MatchText: "BLUE BOTTLE"}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", LinkIDs: []vo.Id{vo.NewId()}}
	fresh, joins := mergeIntoOpen([]model.ImportSuggestion{g}, []model.ImportSuggestion{open})
	require.Len(t, fresh, 1)
	require.Empty(t, joins)
}

func TestOpenForChecksCategoryType(t *testing.T) {
	salary, coffee := vo.NewId(), vo.NewId()
	income := map[vo.Id]bool{salary: true}
	row := sRow(sLink("SQ *BLUE BOTTLE #1", "", time.Time{}))
	o := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "blue bottle", CategoryID: &salary}
	_, ok := openFor(row, []model.ImportSuggestion{o}, income)
	require.False(t, ok)
	o.CategoryID = &coffee
	id, ok := openFor(row, []model.ImportSuggestion{o}, income)
	require.True(t, ok)
	require.Equal(t, o.ID, id)
}

func TestBuildSuggestPromptOmitsOtherData(t *testing.T) {
	v, _, _, _, _ := sVocab()
	row := sRow(sLink("BLUE BOTTLE", "memo", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)))
	row.Link.TransactionID = &row.Link.ID
	sys, user := buildSuggestPrompt([]suggestRow{row}, v, "en", "UTC")
	require.NotEmpty(t, sys)
	var p map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(user), &p))
	require.Len(t, p, 5)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(p["rows"], &rows))
	require.Len(t, rows, 1)
	keys := map[string]bool{}
	for k := range rows[0] {
		keys[k] = true
	}
	require.Equal(t, map[string]bool{"key": true, "payee": true, "description": true, "amount": true, "currency": true, "type": true, "date": true, "account": true}, keys)
	require.False(t, strings.Contains(user, row.Link.ID.String()))
}

type requireT struct{}

var require requireT

func (requireT) True(t *testing.T, v bool) {
	t.Helper()
	if !v {
		t.Fatal("expected true")
	}
}
func (requireT) False(t *testing.T, v bool) {
	t.Helper()
	if v {
		t.Fatal("expected false")
	}
}
func (requireT) NoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func (requireT) Error(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
}
func (requireT) Nil(t *testing.T, v any) {
	t.Helper()
	if v != nil && !reflect.ValueOf(v).IsNil() {
		t.Fatalf("expected nil, got %v", v)
	}
}
func (requireT) NotNil(t *testing.T, v any) {
	t.Helper()
	if v == nil || reflect.ValueOf(v).IsNil() {
		t.Fatal("expected non-nil")
	}
}
func (requireT) NotEmpty(t *testing.T, v any) {
	t.Helper()
	if reflect.ValueOf(v).Len() == 0 {
		t.Fatal("expected non-empty")
	}
}
func (requireT) Empty(t *testing.T, v any) {
	t.Helper()
	if reflect.ValueOf(v).Len() != 0 {
		t.Fatalf("expected empty, got %v", v)
	}
}
func (requireT) Len(t *testing.T, v any, n int) {
	t.Helper()
	if l := reflect.ValueOf(v).Len(); l != n {
		t.Fatalf("len %d, want %d", l, n)
	}
}
func (requireT) Equal(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
