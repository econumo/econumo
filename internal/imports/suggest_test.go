package imports

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"reflect"
)

func sLink(payee, desc string, posted time.Time) model.ImportTransactionLink {
	return model.ImportTransactionLink{ID: vo.NewId(), ExternalPayee: payee, ExternalDescription: desc, ExternalPostedAt: posted}
}

func sRow(link model.ImportTransactionLink) suggestRow {
	r := suggestRow{Link: link, Type: model.TransactionTypeExpense, Currency: "USD", Account: "Card"}
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
	// the description never reaches the AI, so its text is never a match text
	_, _, ok = matchTextIn(sLink("  ", "Coffee at Peets", time.Time{}), "peets")
	require.False(t, ok)
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
	m, err := parseSuggestReply(`{"rows":[{"key":"0","matchText":"x"}]}`)
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

var both = missingFields{Category: true, Payee: true}

// missEach gives every row of every group the same missing fields.
func missEach(groups []model.ImportSuggestion, m missingFields) [][]missingFields {
	out := make([][]missingFields, len(groups))
	for i, g := range groups {
		for range g.LinkIDs {
			out[i] = append(out[i], m)
		}
	}
	return out
}

func merge(g model.ImportSuggestion, m missingFields, open []model.ImportSuggestion, income map[vo.Id]bool) ([][]vo.Id, map[vo.Id][]joinPart) {
	groups := []model.ImportSuggestion{g}
	return mergeIntoOpen(groups, missEach(groups, m), open, income)
}

func joinedLinks(joins map[vo.Id][]joinPart, id vo.Id) []vo.Id {
	var out []vo.Id
	for _, p := range joins[id] {
		out = append(out, p.links...)
	}
	return out
}

func TestMergeIntoOpenJoinsContainedMatchText(t *testing.T) {
	coffee, dining := vo.NewId(), vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", CategoryID: &coffee}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE COFFEE", CategoryID: &dining, LinkIDs: []vo.Id{vo.NewId(), vo.NewId()}}
	rest, joins := merge(g, both, []model.ImportSuggestion{open}, nil)
	require.Empty(t, rest[0])
	require.Equal(t, g.LinkIDs, joinedLinks(joins, open.ID))
}

func TestMergeIntoOpenKeepsDifferentField(t *testing.T) {
	p := vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldDescription, MatchText: "BLUE BOTTLE", PayeeID: &p}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", LinkIDs: []vo.Id{vo.NewId()}}
	rest, joins := merge(g, both, []model.ImportSuggestion{open}, nil)
	require.Len(t, rest[0], 1)
	require.Empty(t, joins)
}

func TestMergeIntoOpenRequiresAFieldTheRowLacks(t *testing.T) {
	p, coffee := vo.NewId(), vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", PayeeID: &p}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", CategoryID: &coffee, LinkIDs: []vo.Id{vo.NewId()}}
	rest, joins := merge(g, missingFields{Category: true}, []model.ImportSuggestion{open}, nil)
	require.Equal(t, g.LinkIDs, rest[0])
	require.Empty(t, joins)
	rest, joins = merge(g, both, []model.ImportSuggestion{open}, nil)
	require.Empty(t, rest[0])
	require.Equal(t, g.LinkIDs, joinedLinks(joins, open.ID))
}

func TestMergeIntoOpenDecidesPerRow(t *testing.T) {
	p, coffee := vo.NewId(), vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", PayeeID: &p}
	lacksPayee, lacksCategory := vo.NewId(), vo.NewId()
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", CategoryID: &coffee, LinkIDs: []vo.Id{lacksPayee, lacksCategory}}
	rest, joins := mergeIntoOpen([]model.ImportSuggestion{g}, [][]missingFields{{{Payee: true}, {Category: true}}}, []model.ImportSuggestion{open}, nil)
	require.Equal(t, []vo.Id{lacksPayee}, joinedLinks(joins, open.ID))
	require.Equal(t, []vo.Id{lacksCategory}, rest[0])
	require.Equal(t, 0, joins[open.ID][0].group)
}

func TestMergeIntoOpenRequiresSameType(t *testing.T) {
	p := vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACME", PayeeID: &p, Type: model.TransactionTypeExpense}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACME", Type: model.TransactionTypeIncome, LinkIDs: []vo.Id{vo.NewId()}}
	rest, joins := merge(g, both, []model.ImportSuggestion{open}, nil)
	require.Len(t, rest[0], 1)
	require.Empty(t, joins)
}

func TestMatchTextInRejectsShort(t *testing.T) {
	l := sLink("COSTCO WHOLESALE", "", time.Time{})
	_, _, ok := matchTextIn(l, "CO")
	require.False(t, ok)
	_, _, ok = matchTextIn(l, "COS")
	require.True(t, ok)
	_, _, ok = matchTextIn(sLink("SQ *COFFEE", "", time.Time{}), "SQ *")
	require.False(t, ok)
}

func TestMergeIntoOpenDoesNotJoinWhenGroupTextShorter(t *testing.T) {
	p := vo.NewId()
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE COFFEE", PayeeID: &p}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BLUE BOTTLE", LinkIDs: []vo.Id{vo.NewId()}}
	rest, joins := merge(g, both, []model.ImportSuggestion{open}, nil)
	require.Len(t, rest[0], 1)
	require.Empty(t, joins)
}

func TestMergeIntoOpenChecksCategoryType(t *testing.T) {
	salary := vo.NewId()
	income := map[vo.Id]bool{salary: true}
	open := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACME", CategoryID: &salary, Type: model.TransactionTypeIncome}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACME CORP", LinkIDs: []vo.Id{vo.NewId()}}
	rest, _ := merge(g, both, []model.ImportSuggestion{open}, income)
	require.Len(t, rest[0], 1)
	g.Type = model.TransactionTypeIncome
	rest, joins := merge(g, both, []model.ImportSuggestion{open}, income)
	require.Empty(t, rest[0])
	require.Len(t, joinedLinks(joins, open.ID), 1)
}

func TestMergeIntoOpenPicksOldest(t *testing.T) {
	now := time.Now()
	p := vo.NewId()
	newer := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACME", PayeeID: &p, CreatedAt: now}
	older := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACM", PayeeID: &p, CreatedAt: now.Add(-time.Hour)}
	g := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "ACME CORP", LinkIDs: []vo.Id{vo.NewId()}}
	_, joins := merge(g, both, []model.ImportSuggestion{newer, older}, nil)
	require.Len(t, joinedLinks(joins, older.ID), 1)
	require.Empty(t, joins[newer.ID])
}

func TestGroupRowsSplitsByTypeAndCarriesMissing(t *testing.T) {
	c := vo.NewId()
	a := validOf(sLink("ACME", "", time.Time{}), "ACME", &c)
	a.Row.Missing = missingFields{Category: true}
	a2 := validOf(sLink("ACME", "", time.Time{}), "ACME", &c)
	a2.Row.Missing = missingFields{Category: true, Payee: true}
	b := validOf(sLink("ACME", "", time.Time{}), "ACME", &c)
	b.Row.Type = model.TransactionTypeIncome
	b.Row.Missing = missingFields{Category: true}
	groups, missing := groupRowsMissing([]validSuggestion{a, a2, b})
	require.Len(t, groups, 2)
	require.Equal(t, model.TransactionTypeExpense, groups[0].Type)
	require.Equal(t, model.TransactionTypeIncome, groups[1].Type)
	require.Equal(t, []missingFields{{Category: true}, {Category: true, Payee: true}}, missing[0])
	require.Equal(t, []missingFields{{Category: true}}, missing[1])
}

func TestLiveOpenDropsArchivedFields(t *testing.T) {
	v, coffee, _, _, bb := sVocab()
	coffeeID, bbID, gone := vo.MustParseId(coffee.ID), vo.MustParseId(bb.ID), vo.NewId()
	open := []model.ImportSuggestion{
		{ID: vo.NewId(), CategoryID: &gone, PayeeID: &bbID},
		{ID: vo.NewId(), CategoryID: &gone},
		{ID: vo.NewId(), CategoryID: &coffeeID},
	}
	got := v.liveOpen(open)
	require.Len(t, got, 2)
	require.Nil(t, got[0].CategoryID)
	require.Equal(t, bbID, *got[0].PayeeID)
	require.Equal(t, coffeeID, *got[1].CategoryID)
	require.Equal(t, gone, *open[0].CategoryID)
}

func TestOpenForRequiresAFieldTheRowLacks(t *testing.T) {
	p := vo.NewId()
	row := sRow(sLink("BLUE BOTTLE", "", time.Time{}))
	row.Missing.Payee = false
	o := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "blue bottle", PayeeID: &p}
	_, ok := openFor(row, []model.ImportSuggestion{o}, nil)
	require.False(t, ok)
	row.Missing.Payee = true
	_, ok = openFor(row, []model.ImportSuggestion{o}, nil)
	require.True(t, ok)
}

func TestOpenForRequiresSameType(t *testing.T) {
	p := vo.NewId()
	row := sRow(sLink("BLUE BOTTLE", "", time.Time{}))
	row.Type = model.TransactionTypeIncome
	o := model.ImportSuggestion{ID: vo.NewId(), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "blue bottle", PayeeID: &p, Type: model.TransactionTypeExpense}
	_, ok := openFor(row, []model.ImportSuggestion{o}, nil)
	require.False(t, ok)
	o.Type = model.TransactionTypeIncome
	_, ok = openFor(row, []model.ImportSuggestion{o}, nil)
	require.True(t, ok)
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

// guardRow is an imported row whose amount, date, description and owner name
// must never reach the AI.
func guardRow() suggestRow {
	l := sLink("BLUE BOTTLE", "memo 4417 lunch with Anna", time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC))
	l.ExternalAmount = "123.45"
	l.TransactionID = &l.ID
	return sRow(l)
}

var guardLeaks = []string{"123.45", "2026-03-02", "03-02", "memo", "4417", "Anna", "Dmitry"}

// jsonKeyPaths lists every key path in a JSON document ("rows[].payee").
func jsonKeyPaths(t *testing.T, raw string) map[string]bool {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	out := map[string]bool{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				path := k
				if prefix != "" {
					path = prefix + "." + k
				}
				out[path] = true
				walk(path, child)
			}
		case []any:
			for _, child := range x {
				walk(prefix+"[]", child)
			}
		}
	}
	walk("", doc)
	return out
}

func keySet(keys ...string) map[string]bool {
	out := map[string]bool{}
	for _, k := range keys {
		out[k] = true
	}
	return out
}

func TestBuildSuggestPromptSendsOnlyTheAllowlist(t *testing.T) {
	v, _, _, _, _ := sVocab()
	row := guardRow()
	sys, user := buildSuggestPrompt([]suggestRow{row}, false, v, "en", "UTC")
	require.NotEmpty(t, sys)
	require.Equal(t, keySet("language", "timezone", "categories", "categories[].id", "categories[].name",
		"rows", "rows[].key", "rows[].payee", "rows[].currency", "rows[].type", "rows[].account"), jsonKeyPaths(t, user))
	var p struct {
		Language, Timezone string
		Rows               []map[string]string
	}
	require.NoError(t, json.Unmarshal([]byte(user), &p))
	require.Equal(t, "en", p.Language)
	require.Equal(t, "UTC", p.Timezone)
	require.Equal(t, []map[string]string{{"key": "0", "payee": "BLUE BOTTLE", "currency": "USD", "type": "expense", "account": "Card"}}, p.Rows)
	for _, leak := range append(guardLeaks, row.Link.ID.String()) {
		require.False(t, strings.Contains(user, leak))
	}
}

func TestBuildPayeePromptSendsOnlyTheAllowlist(t *testing.T) {
	row := guardRow()
	bb := namedList("Blue Bottle Coffee")
	replies := map[string]suggestReplyRow{"0": {MatchText: "BLUE BOTTLE", Merchant: "Blue Bottle"}}
	_, user := buildPayeePrompt([]suggestRow{row}, replies, map[string][]model.ImportNamed{"0": bb}, "en", "UTC")
	require.Equal(t, keySet("language", "timezone", "rows", "rows[].key", "rows[].payee", "rows[].merchant",
		"rows[].candidates", "rows[].candidates[].id", "rows[].candidates[].name"), jsonKeyPaths(t, user))
	var p struct {
		Rows []struct {
			Key, Payee, Merchant string
			Candidates           []model.ImportNamed
		}
	}
	require.NoError(t, json.Unmarshal([]byte(user), &p))
	require.Equal(t, 1, len(p.Rows))
	require.Equal(t, "0", p.Rows[0].Key)
	require.Equal(t, "BLUE BOTTLE", p.Rows[0].Payee)
	require.Equal(t, "Blue Bottle", p.Rows[0].Merchant)
	require.Equal(t, bb[0].ID, p.Rows[0].Candidates[0].ID)
	require.Equal(t, "Blue Bottle Coffee", p.Rows[0].Candidates[0].Name)
	for _, leak := range append(guardLeaks, row.Link.ID.String()) {
		require.False(t, strings.Contains(user, leak))
	}
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

func TestGroupRowsSplitsRowsTheMajorityDoesNotFill(t *testing.T) {
	cat, p := vo.NewId(), vo.NewId()
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// a tie: the newer row (category only) wins; the older row lacks only a
	// payee, which that classification does not fill
	needsPayee := validSuggestion{Row: sRow(sLink("BB", "", day)), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BB", PayeeID: &p}
	needsPayee.Row.Missing = missingFields{Payee: true}
	needsCategory := validSuggestion{Row: sRow(sLink("BB", "", day.AddDate(0, 0, 1))), MatchField: model.ImportRuleMatchFieldExternalPayee, MatchText: "BB", CategoryID: &cat}
	needsCategory.Row.Missing = missingFields{Category: true}
	groups, missing := groupRowsMissing([]validSuggestion{needsPayee, needsCategory})
	require.Len(t, groups, 2)
	require.Equal(t, cat, *groups[0].CategoryID)
	require.Nil(t, groups[0].PayeeID)
	require.Equal(t, []vo.Id{needsCategory.Row.Link.ID}, groups[0].LinkIDs)
	require.Equal(t, p, *groups[1].PayeeID)
	require.Equal(t, []vo.Id{needsPayee.Row.Link.ID}, groups[1].LinkIDs)
	require.Equal(t, []missingFields{{Category: true}}, missing[0])
}

func TestLiveImportUsesOwnersZone(t *testing.T) {
	imported := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	l := model.ImportTransactionLink{ImportedAt: imported, ExternalPostedAt: time.Date(2026, 8, 13, 6, 0, 0, 0, time.UTC)}
	require.True(t, liveImport(l, "America/Los_Angeles"))
	require.False(t, liveImport(l, ""))
	l.ExternalPostedAt = time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	require.False(t, liveImport(l, "Asia/Tokyo"))
	require.True(t, liveImport(l, "Bad/Zone"))
}

func names(list []model.ImportNamed) []string {
	out := []string{}
	for _, n := range list {
		out = append(out, n.Name)
	}
	return out
}

func namedList(ns ...string) []model.ImportNamed {
	out := make([]model.ImportNamed, 0, len(ns))
	for _, n := range ns {
		out = append(out, model.ImportNamed{ID: vo.NewId().String(), Name: n})
	}
	return out
}

func TestBuildSuggestPromptSendsOnlyTheRowTypesCategories(t *testing.T) {
	v, coffee, dining, salary, bb := sVocab()
	day := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	exp := sRow(sLink("BLUE BOTTLE", "", day))
	inc := sRow(sLink("ACME PAYROLL", "", day))
	inc.Type = model.TransactionTypeIncome
	rows := []suggestRow{exp, inc}

	_, user := buildSuggestPrompt(rows, false, v, "en", "UTC")
	for _, want := range []string{coffee.Name, dining.Name, "BLUE BOTTLE", `"key":"0"`} {
		require.True(t, strings.Contains(user, want))
	}
	for _, not := range []string{salary.Name, bb.Name, "ACME", `"key":"1"`} {
		require.False(t, strings.Contains(user, not))
	}

	_, user = buildSuggestPrompt(rows, true, v, "en", "UTC")
	for _, want := range []string{salary.Name, "ACME PAYROLL", `"key":"1"`} {
		require.True(t, strings.Contains(user, want))
	}
	for _, not := range []string{coffee.Name, bb.Name, "BLUE BOTTLE", `"key":"0"`} {
		require.False(t, strings.Contains(user, not))
	}
}

func TestParseSuggestReplyReadsMerchantNotPayee(t *testing.T) {
	m, err := parseSuggestReply(`{"rows":[{"key":"0","matchText":"AMZN","merchant":"Amazon","payeeId":"x"}]}`)
	require.NoError(t, err)
	require.Equal(t, "Amazon", m["0"].Merchant)
	require.Equal(t, "", m["0"].PayeeID)
}

func TestPayeeCandidates(t *testing.T) {
	payees := namedList("Amazon Fresh", "Amazon", "Apple", "Blue Bottle Coffee", "Starbucks", "Trader Joes", "Whole Foods Market")
	for _, tc := range []struct {
		name, merchant, matchText string
		want                      []string
	}{
		{"equality before containment", "Amazon", "AMZN MKTP", []string{"Amazon", "Amazon Fresh"}},
		{"containment", "Blue Bottle", "BLUE BOTTLE", []string{"Blue Bottle Coffee"}},
		{"punctuation and case", "TRADER JOE'S", "TRADER JOE'S #12", []string{"Trader Joes"}},
		{"shared word", "Foods Co", "FOODS CO", []string{"Whole Foods Market"}},
		{"small typo", "Starbuks", "STARBUKS", []string{"Starbucks"}},
		{"match text alone", "Coffee Place", "STARBUCKS", []string{"Starbucks", "Blue Bottle Coffee"}},
		{"nothing alike", "Zara", "ZARA", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, names(payeeCandidates(tc.merchant, tc.matchText, payees)))
		})
	}
}

func TestPayeeCandidatesCapsAtFive(t *testing.T) {
	payees := namedList("Coffee 1", "Coffee 2", "Coffee 3", "Coffee 4", "Coffee 5", "Coffee 6", "Coffee 7", "Coffee")
	got := payeeCandidates("Coffee", "COFFEE", payees)
	require.Len(t, got, 5)
	require.Equal(t, "Coffee", got[0].Name)
}

func TestPayeeCandidatesIgnoreShortContainment(t *testing.T) {
	require.Equal(t, []string{}, names(payeeCandidates("Co", "CO", namedList("Costco"))))
}

func TestVocabularyKeepsEveryPayee(t *testing.T) {
	var all []string
	for i := 0; i < 600; i++ {
		all = append(all, "Filler "+strconv.Itoa(1000+i))
	}
	payees := namedList(append(all, "Zoo Market")...)
	v := newSuggestVocabulary(nil, payees)
	got := payeeCandidates("Zoo Market", "ZOO MARKET", v.Payees)
	require.Equal(t, []string{"Zoo Market"}, names(got))
	_, ok := v.hasPayee(got[0].ID)
	require.True(t, ok)
}

func TestBuildPayeePromptSendsOnlyTheRowsCandidates(t *testing.T) {
	day := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	rows := []suggestRow{sRow(sLink("PHILZ", "", day)), sRow(sLink("SQ *BLUE BOTTLE", "memo", day))}
	bb := namedList("Blue Bottle Coffee")
	replies := map[string]suggestReplyRow{"0": {MatchText: "PHILZ"}, "1": {MatchText: "BLUE BOTTLE", Merchant: "Blue Bottle"}}
	sys, user := buildPayeePrompt(rows, replies, map[string][]model.ImportNamed{"1": bb}, "en", "UTC")
	require.NotEmpty(t, sys)
	for _, want := range []string{"Blue Bottle Coffee", bb[0].ID, "SQ *BLUE BOTTLE", `"merchant":"Blue Bottle"`, `"key":"1"`} {
		require.True(t, strings.Contains(user, want))
	}
	for _, not := range []string{"PHILZ", `"key":"0"`, "categories"} {
		require.False(t, strings.Contains(user, not))
	}
}

func TestParsePayeeReply(t *testing.T) {
	_, err := parsePayeeReply("nope")
	require.Error(t, err)
	_, err = parsePayeeReply(`{"x":1}`)
	require.Error(t, err)
	m, err := parsePayeeReply(`{"rows":[{"key":"1","payeeId":"abc"},{"key":"2"}]}`)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"1": "abc"}, m)
}

func TestApplyPayeeReplyKeepsOnlyTheRowsCandidates(t *testing.T) {
	a, b := namedList("Alpha"), namedList("Beta")
	replies := map[string]suggestReplyRow{"0": {MatchText: "x"}, "1": {MatchText: "y"}}
	applyPayeeReply(replies, map[string]string{"0": b[0].ID, "1": b[0].ID, "2": a[0].ID}, map[string][]model.ImportNamed{"0": a, "1": b})
	require.Equal(t, "", replies["0"].PayeeID)
	require.Equal(t, b[0].ID, replies["1"].PayeeID)
	_, added := replies["2"]
	require.False(t, added)
}

// Providers often wrap the JSON in a code fence or a sentence.
func TestParseRepliesReadFencedJSON(t *testing.T) {
	m, err := parseSuggestReply("```json\n{\"rows\":[{\"key\":\"0\",\"matchText\":\"x\"}]}\n```")
	require.NoError(t, err)
	require.Equal(t, "x", m["0"].MatchText)
	p, err := parsePayeeReply("Here you go:\n```json\n{\"rows\":[{\"key\":\"1\",\"payeeId\":\"abc\"}]}\n```")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"1": "abc"}, p)
	_, err = parseSuggestReply("```json\n```")
	require.Error(t, err)
}

// A row posted in the future is not a live import, beyond a day of clock or
// zone skew.
func TestLiveImportRejectsFutureRows(t *testing.T) {
	imported := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	l := model.ImportTransactionLink{ImportedAt: imported, ExternalPostedAt: imported.Add(20 * time.Hour)}
	require.True(t, liveImport(l, "UTC"))
	l.ExternalPostedAt = imported.Add(25 * time.Hour)
	require.False(t, liveImport(l, "UTC"))
	l.ExternalPostedAt = imported.Add(30 * 24 * time.Hour)
	require.False(t, liveImport(l, "UTC"))
}

// SimpleFIN copies the description into the payee when the bridge sends
// none: such a payee is the memo, which never goes to the AI.
func TestAIPayeeIgnoresACopiedDescription(t *testing.T) {
	require.Equal(t, "", aiPayee(sLink("Transfer to Anna rent 4417", "Transfer to Anna rent 4417", time.Time{})))
	require.Equal(t, "", aiPayee(sLink("transfer to anna  RENT 4417", " Transfer to Anna rent 4417", time.Time{})))
	long := strings.Repeat("x", 300)
	require.Equal(t, "", aiPayee(sLink(string([]rune(long)[:255]), long, time.Time{})))
	require.Equal(t, "", aiPayee(sLink("   ", "memo", time.Time{})))
	require.Equal(t, "BLUE BOTTLE", aiPayee(sLink("BLUE BOTTLE", "BLUE BOTTLE #42 coffee", time.Time{})))
	require.Equal(t, "BLUE BOTTLE", aiPayee(sLink("BLUE BOTTLE", "", time.Time{})))
	_, _, ok := matchTextIn(sLink("Transfer to Anna rent", "Transfer to Anna rent", time.Time{}), "Anna")
	require.False(t, ok)
}

func TestReplySchemasFitStructuredOutputs(t *testing.T) {
	for _, s := range []*ai.Schema{suggestSchema, payeeSchema, matchTextSchema} {
		require.NoError(t, s.Check())
	}
}

func TestParseSuggestReplyNullsMissingAndRepeatedRows(t *testing.T) {
	m, err := parseSuggestReply(`{"rows":[{"key":"0","matchText":"AMZN","merchant":null,"categoryId":null},{"key":"0","matchText":"other"},{"key":" 2 ","matchText":"Z","merchant":"Zed","categoryId":"c"},{"matchText":"no key"}]}`)
	require.NoError(t, err)
	require.Equal(t, map[string]suggestReplyRow{
		"0": {Key: "0", MatchText: "AMZN"},
		"2": {Key: "2", MatchText: "Z", Merchant: "Zed", CategoryID: "c"},
	}, m)
	m, err = parseSuggestReply(`{"rows":[]}`)
	require.NoError(t, err)
	require.Empty(t, m)
}

func TestParsePayeeReplyNullsAndRepeatedRows(t *testing.T) {
	m, err := parsePayeeReply(`{"rows":[{"key":"0","payeeId":null},{"key":"1","payeeId":"a"},{"key":"1","payeeId":"b"}]}`)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"1": "a"}, m)
}
