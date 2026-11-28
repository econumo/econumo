//go:build aieval

package transaction_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/aieval"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

// The AI eval suite of the CSV column suggestion: real bank export headers go
// to the provider in ECONUMO_AI_DSN through the production use case. The model
// should leave a role empty rather than guess, so a case passes when every
// role it assigns is right and the money and date roles the case expects
// (date, amount, amountIn, amountOut, all clearly named) are assigned. An
// optional role left empty is not a failure; a wrong assignment is.

// Column types as the browser computes them, one letter per column: d date,
// n number, N number with negatives, t text, e empty. A want key "a|b" accepts
// the header on either role, for a column both readings fit.
type columnsCase struct {
	headers string
	types   string
	want    map[string]string
}

var columnTypeNames = map[byte]string{'d': "date", 'n': "number", 'N': "number", 't': "text", 'e': "empty"}

func (c columnsCase) request(sep string) model.SuggestImportColumnsRequest {
	names := strings.Split(c.headers, sep)
	if len(names) != len(c.types) {
		panic(fmt.Sprintf("columns case %q: %d headers, %d types", c.headers, len(names), len(c.types)))
	}
	req := model.SuggestImportColumnsRequest{}
	for i, name := range names {
		req.Columns = append(req.Columns, model.ImportColumnFact{Name: name, Type: columnTypeNames[c.types[i]], Negatives: c.types[i] == 'N'})
	}
	return req
}

var evalColumnCases = map[string][]columnsCase{
	"en": {
		{"Transaction Date,Post Date,Description,Category,Type,Amount,Memo", "ddtttNe", map[string]string{"date": "Transaction Date", "amount": "Amount", "description": "Description", "category": "Category"}},
		{"Date,Description,Debit,Credit,Balance", "dtnnn", map[string]string{"date": "Date", "description": "Description", "amountOut": "Debit", "amountIn": "Credit"}},
		{"Posting Date,Details,Description,Amount,Type,Balance,Check or Slip #", "dttNtne", map[string]string{"date": "Posting Date", "amount": "Amount", "description": "Description"}},
		{"Date,Payee,Notes,Category,Tags,Amount,Currency,Account", "dttttNtt", map[string]string{"date": "Date", "payee": "Payee", "description": "Notes", "category": "Category", "tag": "Tags", "amount": "Amount", "account": "Account"}},
		{"Booking Date,Value Date,Merchant,Details,Money Out,Money In,Balance", "ddttnnn", map[string]string{"date": "Booking Date", "payee": "Merchant", "description": "Details", "amountOut": "Money Out", "amountIn": "Money In"}},
		{"Date,Name,Memo,Withdrawals,Deposits", "dttnn", map[string]string{"date": "Date", "payee": "Name", "description": "Memo", "amountOut": "Withdrawals", "amountIn": "Deposits"}},
		{"Date,Account,Merchant,Category,Labels,Amount", "dttttN", map[string]string{"date": "Date", "account": "Account", "payee": "Merchant", "category": "Category", "labels": "Labels", "amount": "Amount"}},
		{"Settled Date,Reference,Transaction Description,Transaction Amount,Running Balance", "dttNn", map[string]string{"date": "Settled Date", "description": "Transaction Description", "amount": "Transaction Amount"}},
		{"Date,Narration,Chq./Ref.No.,Value Dt,Withdrawal Amt.,Deposit Amt.,Closing Balance", "dttdnnn", map[string]string{"date": "Date", "description": "Narration", "amountOut": "Withdrawal Amt.", "amountIn": "Deposit Amt."}},
	},
	"de": {
		{"Buchungstag;Valutadatum;Buchungstext;Auftraggeber / Begünstigter;Verwendungszweck;Kontonummer;BLZ;Betrag (EUR)", "ddttttnN", map[string]string{"date": "Buchungstag", "amount": "Betrag (EUR)", "description": "Verwendungszweck", "payee": "Auftraggeber / Begünstigter"}},
		{"Datum;Name;Verwendungszweck;Soll;Haben;Saldo", "dttnnn", map[string]string{"date": "Datum", "payee": "Name", "description": "Verwendungszweck", "amountOut": "Soll", "amountIn": "Haben"}},
		{"Buchungsdatum;Wertstellung;Status;Zahlungspflichtige*r;Zahlungsempfänger*in;Verwendungszweck;Umsatztyp;IBAN;Betrag (€);Gläubiger-ID;Mandatsreferenz", "ddttttttNtt", map[string]string{"date": "Buchungsdatum", "amount": "Betrag (€)", "description": "Verwendungszweck"}},
		{"Bezeichnung Auftragskonto;IBAN Auftragskonto;Buchungstag;Valutadatum;Name Zahlungsbeteiligter;Buchungstext;Verwendungszweck;Betrag;Waehrung", "ttddtttNt", map[string]string{"date": "Buchungstag", "payee": "Name Zahlungsbeteiligter", "description": "Verwendungszweck", "amount": "Betrag", "account": "Bezeichnung Auftragskonto"}},
		{"Datum;Beschreibung;Kategorie;Betrag;Konto;Tags", "dttNtt", map[string]string{"date": "Datum", "description": "Beschreibung", "category": "Kategorie", "amount": "Betrag", "account": "Konto", "tag": "Tags"}},
		{"Belegdatum;Empfänger;Buchungstext;Ausgang;Eingang", "dttnn", map[string]string{"date": "Belegdatum", "payee": "Empfänger", "description": "Buchungstext", "amountOut": "Ausgang", "amountIn": "Eingang"}},
		{"Tag;Valuta;Beguenstigter/Zahlungspflichtiger;Verwendungszweck;Umsatz;Saldo", "ddttNn", map[string]string{"date": "Tag", "payee": "Beguenstigter/Zahlungspflichtiger", "description": "Verwendungszweck", "amount": "Umsatz"}},
		{"Datum;Uhrzeit;Zeitzone;Name;Typ;Status;Währung;Brutto;Gebühr;Netto;Transaktionscode;Betreff", "dttttttNNNtt", map[string]string{"date": "Datum", "payee": "Name", "description": "Betreff", "amount": "Netto"}},
	},
	"ru": {
		{"Дата операции;Дата платежа;Номер карты;Статус;Сумма операции;Валюта операции;Категория;Описание", "ddttNttt", map[string]string{"date": "Дата операции", "amount": "Сумма операции", "category": "Категория", "description": "Описание"}},
		{"Дата;Описание;Приход;Расход;Остаток", "dtnnn", map[string]string{"date": "Дата", "description": "Описание", "amountIn": "Приход", "amountOut": "Расход"}},
		{"Дата и время;Тип операции;Получатель;Назначение платежа;Сумма;Валюта;Счёт", "dtttNtt", map[string]string{"date": "Дата и время", "payee": "Получатель", "description": "Назначение платежа", "amount": "Сумма", "account": "Счёт"}},
		{"Дата операции;Дата списания;Сумма в валюте счета;Валюта счета;Сумма в валюте операции;Валюта операции;Описание;MCC;Категория", "ddNtNttnt", map[string]string{"date": "Дата операции", "amount": "Сумма в валюте счета", "description": "Описание", "category": "Категория"}},
		{"Дата проводки;Контрагент;Назначение;Дебет;Кредит", "dttnn", map[string]string{"date": "Дата проводки", "payee": "Контрагент", "description": "Назначение", "amountOut": "Дебет", "amountIn": "Кредит"}},
		{"Дата;Сумма;Категория;Метки;Комментарий;Счёт", "dNtttt", map[string]string{"date": "Дата", "amount": "Сумма", "category": "Категория", "labels": "Метки", "description": "Комментарий", "account": "Счёт"}},
		{"Дата;Наименование;Сумма платежа;Баланс", "dtNn", map[string]string{"date": "Дата", "description|payee": "Наименование", "amount": "Сумма платежа"}},
		{"Дата транзакции;Магазин;Сумма;Валюта;Теги;Заметка", "dtNttt", map[string]string{"date": "Дата транзакции", "payee": "Магазин", "amount": "Сумма", "tag|labels": "Теги", "description": "Заметка"}},
		{"Дата;Расход;Доход;Категория;Комментарий", "dnntt", map[string]string{"date": "Дата", "amountOut": "Расход", "amountIn": "Доход", "category": "Категория", "description": "Комментарий"}},
	},
}

var evalColumnSeparators = map[string]string{"en": ",", "de": ";", "ru": ";"}

// The column types each role accepts, as the prompt states them; a case whose
// types contradict its expected roles grades the fixture, not the model.
var columnRoleTypes = map[string]string{"date": "d", "amount": "nN", "amountIn": "nN", "amountOut": "nN", "description": "t", "payee": "t", "category": "t", "tag": "t", "labels": "t", "account": "t"}

// Roles a case must get back whenever it expects them, and must never get when it does not.
var requiredColumnRoles = map[string]bool{"date": true, "amount": true, "amountIn": true, "amountOut": true}

// gradeColumns: an assigned role is wrong when the case expects that role on
// another header, when its header is expected for other roles only, or when it
// is a required role the case does not expect. An optional role on a header the
// case says nothing about is tolerated: the fixtures do not list every sound
// reading of a header (a "Details" column read as the payee).
func gradeColumns(want, got map[string]string) bool {
	expected := map[string][]string{}
	rolesOf := map[string][]string{}
	for roles, header := range want {
		for _, role := range strings.Split(roles, "|") {
			expected[role] = append(expected[role], header)
			rolesOf[header] = append(rolesOf[header], role)
		}
	}
	for role, header := range got {
		if headers, ok := expected[role]; ok {
			if !slices.Contains(headers, header) {
				return false
			}
			continue
		}
		if requiredColumnRoles[role] || len(rolesOf[header]) > 0 {
			return false
		}
	}
	for roles, header := range want {
		required := false
		matched := false
		for _, role := range strings.Split(roles, "|") {
			required = required || requiredColumnRoles[role]
			matched = matched || got[role] == header
		}
		if required && !matched {
			return false
		}
	}
	return true
}

func TestAIEval_ColumnFixturesAreConsistent(t *testing.T) {
	for lang, cases := range evalColumnCases {
		for _, c := range cases {
			headers := strings.Split(c.headers, evalColumnSeparators[lang])
			if len(headers) != len(c.types) {
				t.Errorf("%s %q: %d headers, %d types", lang, c.headers, len(headers), len(c.types))
				continue
			}
			for roles, want := range c.want {
				column := -1
				for i, h := range headers {
					if h == want {
						column = i
					}
				}
				if column < 0 {
					t.Errorf("%s %q: role %s names %q, not a header", lang, c.headers, roles, want)
					continue
				}
				for _, role := range strings.Split(roles, "|") {
					if fits := columnRoleTypes[role]; fits != "" && !strings.ContainsRune(fits, rune(c.types[column])) {
						t.Errorf("%s %q: role %s on %q, a column of type %c", lang, c.headers, role, want, c.types[column])
					}
				}
			}
		}
	}
}

func TestAIEval_ColumnGrading(t *testing.T) {
	want := map[string]string{"date": "Date", "amount": "Amount", "description": "Memo", "tag|labels": "Tags"}
	cases := []struct {
		name string
		got  map[string]string
		pass bool
	}{
		{"everything", map[string]string{"date": "Date", "amount": "Amount", "description": "Memo", "labels": "Tags"}, true},
		{"optional roles left empty", map[string]string{"date": "Date", "amount": "Amount"}, true},
		{"an optional role on an unlisted header", map[string]string{"date": "Date", "amount": "Amount", "payee": "Name"}, true},
		{"a required role missing", map[string]string{"date": "Date", "description": "Memo"}, false},
		{"an optional role on the wrong header", map[string]string{"date": "Date", "amount": "Amount", "description": "Name"}, false},
		{"an expected header on another role", map[string]string{"date": "Date", "amount": "Amount", "payee": "Memo"}, false},
		{"an unexpected required role", map[string]string{"date": "Date", "amount": "Amount", "amountIn": "Balance"}, false},
	}
	for _, c := range cases {
		if got := gradeColumns(want, c.got); got != c.pass {
			t.Errorf("%s: pass = %v, want %v", c.name, got, c.pass)
		}
	}
}

func TestAIEval_ImportColumns(t *testing.T) {
	provider := aieval.Provider(t)
	defer provider.Spend(t)
	var areas []*aieval.Area
	for _, lang := range []string{"en", "de", "ru"} {
		area := aieval.NewArea("import-columns/" + lang)
		areas = append(areas, area)
		svc := apptransaction.NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		svc.SetColumnAdvisor(provider, columnsFacts{info: apptransaction.AIFactsInfo{Language: lang}}, nil)
		for _, c := range evalColumnCases[lang] {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			res, err := svc.SuggestImportColumns(ctx, columnsUser, c.request(evalColumnSeparators[lang]))
			cancel()
			if err != nil {
				t.Errorf("%s: %v", area.Name, err)
				continue
			}
			area.Check(gradeColumns(c.want, res.Roles), c.headers, fmt.Sprint(res.Roles), fmt.Sprint(c.want))
		}
	}
	aieval.Finish(t, areas...)
}
