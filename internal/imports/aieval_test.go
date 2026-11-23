//go:build aieval

package imports

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/aieval"
)

// The AI eval suite of the import prompts: it sends the real prompts to the
// provider in ECONUMO_AI_DSN (both suggestion steps, batched per language as
// the job batches an owner's rows, and the rule editor's match text one row
// at a time) and scores what the production validators keep.

type evalVocabulary struct {
	expense, income, payees []string
}

var evalVocab = map[string]evalVocabulary{
	"en": {
		expense: []string{"Groceries", "Restaurants", "Coffee", "Transport", "Fuel", "Shopping", "Subscriptions", "Utilities", "Health", "Entertainment", "Travel", "Fees"},
		income:  []string{"Salary", "Refunds", "Interest"},
		payees:  []string{"Amazon", "Starbucks", "Tim Hortons", "Costco", "Uber", "Netflix", "Shell", "Loblaws", "Walmart", "Apple", "Spotify", "Metro", "Shoppers Drug Mart", "Joe's Pizza"},
	},
	"ru": {
		expense: []string{"Продукты", "Рестораны и кафе", "Кофе", "Транспорт", "Такси", "Топливо", "Покупки", "Подписки", "Коммунальные услуги", "Здоровье", "Развлечения", "Путешествия", "Комиссии"},
		income:  []string{"Зарплата", "Возвраты", "Проценты"},
		payees:  []string{"Пятёрочка", "ВкусВилл", "Магнит", "Яндекс Такси", "Яндекс Плюс", "Ozon", "Wildberries", "Перекрёсток", "Лукойл", "Аптека Ригла", "Шоколадница"},
	},
}

// evalRow is one imported transaction. match lists acceptable match texts
// (nil: any text the validator keeps; not scored when category and payee
// both expect none); category and payee list acceptable names, "" = none.
type evalRow struct {
	payee    string
	income   bool
	match    []string
	category []string
	payee2   []string
}

var none = []string{""}

var evalSuggestCases = map[string][]evalRow{
	"en": {
		{"AMZN Mktp CA*2K4HJ8XY1", false, []string{"AMZN Mktp CA", "AMZN Mktp", "AMZN"}, []string{"Shopping", ""}, []string{"Amazon"}},
		{"Amazon.ca*RT5YU7", false, []string{"Amazon.ca", "Amazon"}, []string{"Shopping", ""}, []string{"Amazon"}},
		{"SQ *BLUE BOTTLE COFFEE", false, []string{"BLUE BOTTLE COFFEE", "BLUE BOTTLE"}, []string{"Coffee"}, none},
		{"TST* JOES PIZZA 0042", false, []string{"JOES PIZZA"}, []string{"Restaurants"}, []string{"Joe's Pizza"}},
		{"STARBUCKS #4521 TORONTO ON", false, []string{"STARBUCKS"}, []string{"Coffee"}, []string{"Starbucks"}},
		{"TIM HORTONS #2231", false, []string{"TIM HORTONS"}, []string{"Coffee", "Restaurants"}, []string{"Tim Hortons"}},
		{"COSTCO WHOLESALE W532", false, []string{"COSTCO WHOLESALE", "COSTCO"}, []string{"Groceries", "Shopping"}, []string{"Costco"}},
		{"UBER *TRIP HELP.UBER.COM", false, []string{"UBER *TRIP", "UBER"}, []string{"Transport"}, []string{"Uber"}},
		{"UBER *EATS PENDING", false, []string{"UBER *EATS", "UBER"}, []string{"Restaurants"}, []string{"Uber", ""}},
		{"NETFLIX.COM", false, []string{"NETFLIX.COM", "NETFLIX"}, []string{"Subscriptions", "Entertainment"}, []string{"Netflix"}},
		{"SHELL C12345 OAKVILLE", false, []string{"SHELL"}, []string{"Fuel"}, []string{"Shell"}},
		{"LOBLAWS 1029", false, []string{"LOBLAWS"}, []string{"Groceries"}, []string{"Loblaws"}},
		{"WAL-MART #3045", false, []string{"WAL-MART"}, []string{"Groceries", "Shopping"}, []string{"Walmart"}},
		{"APPLE.COM/BILL", false, []string{"APPLE.COM/BILL", "APPLE.COM", "APPLE"}, []string{"Subscriptions", "Entertainment", "Shopping", ""}, []string{"Apple"}},
		{"SPOTIFY P1A2B3C4", false, []string{"SPOTIFY"}, []string{"Subscriptions", "Entertainment"}, []string{"Spotify"}},
		{"SHOPPERS DRUG MART #0812", false, []string{"SHOPPERS DRUG MART", "SHOPPERS"}, []string{"Health", "Shopping"}, []string{"Shoppers Drug Mart"}},
		{"METRO 712 TORONTO", false, []string{"METRO"}, []string{"Groceries"}, []string{"Metro"}},
		{"TORONTO HYDRO", false, []string{"TORONTO HYDRO"}, []string{"Utilities"}, none},
		{"AIR CANADA 0142345678901", false, []string{"AIR CANADA"}, []string{"Travel"}, none},
		{"PRESTO AUTOLOAD", false, []string{"PRESTO"}, []string{"Transport"}, none},
		{"DOORDASH*THAI EXPRESS", false, []string{"DOORDASH*THAI EXPRESS", "DOORDASH", "THAI EXPRESS"}, []string{"Restaurants"}, none},
		{"SQ *FARMERS MARKET", false, []string{"FARMERS MARKET"}, []string{"Groceries"}, none},
		{"MONTHLY ACCOUNT FEE", false, nil, []string{"Fees", ""}, none},
		{"POS PURCHASE 4829 XKQ", false, nil, none, none},
		{"E-TRANSFER 3920193", false, nil, none, none},
		{"AMAZON.CA REFUND", true, []string{"AMAZON.CA", "AMAZON"}, []string{"Refunds"}, []string{"Amazon"}},
		{"COSTCO RETURN W532", true, []string{"COSTCO"}, []string{"Refunds"}, []string{"Costco"}},
		{"ACME CORP PAYROLL", true, []string{"ACME CORP PAYROLL", "ACME CORP", "ACME"}, []string{"Salary"}, none},
		{"INTEREST PAID", true, nil, []string{"Interest", ""}, none},
		{"CRA GST CREDIT", true, nil, []string{"", "Refunds"}, none},
	},
	"ru": {
		{"ПЯТЕРОЧКА 1234 МОСКВА", false, []string{"ПЯТЕРОЧКА"}, []string{"Продукты"}, []string{"Пятёрочка"}},
		{"YANDEX*TAXI", false, []string{"YANDEX*TAXI"}, []string{"Такси", "Транспорт"}, []string{"Яндекс Такси"}},
		{"ВКУСВИЛЛ", false, []string{"ВКУСВИЛЛ"}, []string{"Продукты"}, []string{"ВкусВилл"}},
		{"МАГНИТ М-1234", false, []string{"МАГНИТ"}, []string{"Продукты"}, []string{"Магнит"}},
		{"OZON.RU", false, []string{"OZON.RU", "OZON"}, []string{"Покупки"}, []string{"Ozon"}},
		{"WILDBERRIES", false, []string{"WILDBERRIES"}, []string{"Покупки"}, []string{"Wildberries"}},
		{"YANDEX*PLUS", false, []string{"YANDEX*PLUS"}, []string{"Подписки", "Развлечения"}, []string{"Яндекс Плюс"}},
		{"PEREKRESTOK 0567 MOSCOW", false, []string{"PEREKRESTOK"}, []string{"Продукты"}, []string{"Перекрёсток"}},
		{"LUKOIL AZS 77123", false, []string{"LUKOIL", "LUKOIL AZS"}, []string{"Топливо"}, []string{"Лукойл"}},
		{"АПТЕКА РИГЛА 112", false, []string{"АПТЕКА РИГЛА", "РИГЛА"}, []string{"Здоровье"}, []string{"Аптека Ригла"}},
		{"SHOKOLADNICA 0021", false, []string{"SHOKOLADNICA"}, []string{"Рестораны и кафе", "Кофе"}, []string{"Шоколадница"}},
		{"МОСЭНЕРГОСБЫТ", false, []string{"МОСЭНЕРГОСБЫТ"}, []string{"Коммунальные услуги"}, none},
		{"MOSCOW METRO", false, []string{"MOSCOW METRO", "METRO"}, []string{"Транспорт"}, none},
		{"AEROFLOT 5552100", false, []string{"AEROFLOT"}, []string{"Путешествия"}, none},
		{"KINO MORI CINEMA", false, []string{"KINO MORI CINEMA", "KINO MORI", "MORI CINEMA"}, []string{"Развлечения"}, none},
		{"SAMOKAT", false, []string{"SAMOKAT"}, []string{"Продукты"}, none},
		{"КОФЕМАНИЯ", false, []string{"КОФЕМАНИЯ"}, []string{"Кофе", "Рестораны и кафе"}, none},
		{"ЛЕНТА 0145", false, []string{"ЛЕНТА"}, []string{"Продукты"}, none},
		{"DNS SHOP 3301", false, []string{"DNS SHOP", "DNS"}, []string{"Покупки"}, none},
		{"MTS MOBILE", false, nil, []string{"Коммунальные услуги", "Подписки", ""}, none},
		{"SBERMARKET", false, []string{"SBERMARKET"}, []string{"Продукты", "Покупки"}, none},
		{"КОМИССИЯ ЗА ОБСЛУЖИВАНИЕ", false, nil, []string{"Комиссии", ""}, none},
		{"ОПЛАТА ПО QR 8812", false, nil, none, none},
		{"СБЕР ПЕРЕВОД", false, nil, none, none},
		{"ВОЗВРАТ OZON.RU", true, []string{"OZON.RU", "OZON"}, []string{"Возвраты"}, []string{"Ozon"}},
		{"ВОЗВРАТ WILDBERRIES", true, []string{"WILDBERRIES"}, []string{"Возвраты"}, []string{"Wildberries"}},
		{"ЗАРПЛАТА ООО РОМАШКА", true, []string{"ООО РОМАШКА", "РОМАШКА", "ЗАРПЛАТА ООО РОМАШКА"}, []string{"Зарплата"}, none},
		{"ПРОЦЕНТЫ ПО ВКЛАДУ", true, nil, []string{"Проценты", ""}, none},
		{"КЭШБЭК", true, nil, []string{"Возвраты", "Проценты", ""}, none},
		{"ПЕРЕВОД ОТ ИВАН И.", true, nil, none, none},
	},
}

// evalMatchCases: the rule editor's match text; nil = no merchant.
var evalMatchCases = map[string][]struct {
	payee string
	match []string
}{
	"en": {
		{"AMZN Mktp CA*2K4HJ8XY1", []string{"AMZN Mktp CA", "AMZN Mktp", "AMZN"}},
		{"SQ *BLUE BOTTLE COFFEE", []string{"BLUE BOTTLE COFFEE", "BLUE BOTTLE"}},
		{"TST* JOES PIZZA 0042", []string{"JOES PIZZA"}},
		{"STARBUCKS #4521 TORONTO ON", []string{"STARBUCKS"}},
		{"SHELL C12345 OAKVILLE", []string{"SHELL"}},
		{"LOBLAWS 1029", []string{"LOBLAWS"}},
		{"UBER *TRIP HELP.UBER.COM", []string{"UBER *TRIP", "UBER"}},
		{"NETFLIX.COM", []string{"NETFLIX.COM", "NETFLIX"}},
		{"PAYPAL *STEAMGAMES", []string{"STEAMGAMES", "PAYPAL *STEAMGAMES"}},
		{"COSTCO WHOLESALE W532", []string{"COSTCO WHOLESALE", "COSTCO"}},
		{"DOORDASH*THAI EXPRESS", []string{"DOORDASH*THAI EXPRESS", "DOORDASH", "THAI EXPRESS"}},
		{"WAL-MART #3045", []string{"WAL-MART"}},
		{"TIM HORTONS #2231", []string{"TIM HORTONS"}},
		{"SHOPPERS DRUG MART #0812", []string{"SHOPPERS DRUG MART", "SHOPPERS"}},
		{"POS PURCHASE 4829 XKQ", nil},
		{"E-TRANSFER 3920193", nil},
	},
	"ru": {
		{"ПЯТЕРОЧКА 1234 МОСКВА", []string{"ПЯТЕРОЧКА"}},
		{"YANDEX*TAXI", []string{"YANDEX*TAXI"}},
		{"МАГНИТ М-1234", []string{"МАГНИТ"}},
		{"ВКУСВИЛЛ", []string{"ВКУСВИЛЛ"}},
		{"OZON.RU", []string{"OZON.RU", "OZON"}},
		{"WILDBERRIES", []string{"WILDBERRIES"}},
		{"LUKOIL AZS 77123", []string{"LUKOIL", "LUKOIL AZS"}},
		{"АПТЕКА РИГЛА 112", []string{"АПТЕКА РИГЛА", "РИГЛА"}},
		{"PEREKRESTOK 0567 MOSCOW", []string{"PEREKRESTOK"}},
		{"ЛЕНТА 0145", []string{"ЛЕНТА"}},
		{"YANDEX*PLUS", []string{"YANDEX*PLUS"}},
		{"DNS SHOP 3301", []string{"DNS SHOP", "DNS"}},
		{"KINO MORI CINEMA", []string{"KINO MORI CINEMA", "KINO MORI", "MORI CINEMA"}},
		{"СБЕР ПЕРЕВОД", nil},
		{"ОПЛАТА ПО QR 8812", nil},
		{"ПЕРЕВОД ОТ ИВАН И.", nil},
	},
}

var evalAccountName = map[string]string{"en": "Visa Infinite", "ru": "Тинькофф Black"}
var evalCurrency = map[string]string{"en": "CAD", "ru": "RUB"}

type evalFacts struct{}

func (evalFacts) OwnerFacts(context.Context, vo.Id) (OwnerInfo, error) {
	return OwnerInfo{SuggestRules: true, SuggestFill: true}, nil
}

func evalContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func evalNamed(names []string, income bool) ([]model.ImportNamed, map[string]string) {
	out := make([]model.ImportNamed, 0, len(names))
	byID := map[string]string{}
	for _, n := range names {
		id := vo.NewId().String()
		out = append(out, model.ImportNamed{ID: id, Name: n, Income: income})
		byID[id] = n
	}
	return out, byID
}

func idName(id *vo.Id, names map[string]string) string {
	if id == nil {
		return ""
	}
	return names[id.String()]
}

func TestAIEval_Suggestions(t *testing.T) {
	provider := aieval.Provider(t)
	defer provider.Spend(t)
	owner := vo.NewId()
	var areas []*aieval.Area
	for _, lang := range aieval.Languages {
		matchArea := aieval.NewArea("suggest-match/" + lang.Code)
		categoryArea := aieval.NewArea("suggest-category/" + lang.Code)
		payeeArea := aieval.NewArea("suggest-payee/" + lang.Code)
		areas = append(areas, matchArea, categoryArea, payeeArea)

		voc := evalVocab[lang.Code]
		expense, names := evalNamed(voc.expense, false)
		income, incomeNames := evalNamed(voc.income, true)
		for id, n := range incomeNames {
			names[id] = n
		}
		payees, payeeNames := evalNamed(voc.payees, false)
		sort.SliceStable(payees, func(i, j int) bool { return strings.ToLower(payees[i].Name) < strings.ToLower(payees[j].Name) })
		vocab := newSuggestVocabulary(append(expense, income...), payees)

		cases := evalSuggestCases[lang.Code]
		rows := make([]suggestRow, len(cases))
		posted := time.Now().UTC().Add(-24 * time.Hour)
		for i, c := range cases {
			typ := model.TransactionTypeExpense
			if c.income {
				typ = model.TransactionTypeIncome
			}
			rows[i] = suggestRow{
				Link:     model.ImportTransactionLink{ID: vo.NewId(), ExternalPayee: c.payee, ExternalPostedAt: posted},
				Type:     typ,
				Currency: evalCurrency[lang.Code],
				Account:  evalAccountName[lang.Code],
				Missing:  missingFields{Category: true, Payee: true},
			}
		}
		svc := &Service{completer: provider, facts: evalFacts{}}
		replies, err := svc.askSuggestions(evalContext(t), owner, rows, vocab, lang.Code, lang.Timezone)
		if err != nil {
			t.Errorf("%s: %v", lang.Code, err)
			continue
		}
		kept := map[vo.Id]validSuggestion{}
		for _, s := range validateReply(rows, replies, vocab) {
			kept[s.Row.Link.ID] = s
		}
		for i, c := range cases {
			reply := replies[strconv.Itoa(i)]
			s := kept[rows[i].Link.ID]
			gotCategory, gotPayee := idName(s.CategoryID, names), idName(s.PayeeID, payeeNames)
			categoryArea.Check(aieval.OneOf(gotCategory, c.category), c.payee, aieval.Show(gotCategory), aieval.ShowSet(c.category))
			payeeArea.Check(aieval.OneOf(gotPayee, c.payee2), c.payee, aieval.Show(gotPayee), aieval.ShowSet(c.payee2))
			if aieval.OneOf("", c.category) && aieval.OneOf("", c.payee2) && len(c.match) == 0 {
				continue
			}
			_, text, ok := matchTextIn(rows[i].Link, reply.MatchText)
			want := "a substring of the payee"
			pass := ok
			if len(c.match) > 0 {
				want = aieval.ShowSet(c.match)
				pass = ok && aieval.OneOf(text, c.match)
			}
			got := aieval.Show(reply.MatchText)
			if !ok && reply.MatchText != "" {
				got += " (refused by the validator)"
			}
			matchArea.Check(pass, c.payee, got, want)
		}
	}
	aieval.Finish(t, areas...)
}

func TestAIEval_MatchText(t *testing.T) {
	provider := aieval.Provider(t)
	defer provider.Spend(t)
	svc := &Service{completer: provider, facts: evalFacts{}}
	var areas []*aieval.Area
	for _, lang := range aieval.Languages {
		area := aieval.NewArea("match-text/" + lang.Code)
		areas = append(areas, area)
		for _, c := range evalMatchCases[lang.Code] {
			link := model.ImportTransactionLink{ID: vo.NewId(), ExternalPayee: c.payee}
			_, text, ok := svc.askMatchText(evalContext(t), "eval", link, lang.Code, lang.Timezone)
			if !ok {
				text = ""
			}
			want := c.match
			if want == nil {
				want = none
			}
			area.Check(aieval.OneOf(text, want), c.payee, aieval.Show(text), aieval.ShowSet(want))
		}
	}
	aieval.Finish(t, areas...)
}
