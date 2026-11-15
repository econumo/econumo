//go:build aieval

package onboarding

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/aieval"
)

// The AI eval suite of the onboarding prompts: it sends the real prompts to
// the provider in ECONUMO_AI_DSN, one batched request per area and language as
// production does, and scores the replies the production parsers keep.

type evalAccount struct {
	en, ru, currency string
	savings          bool
}

var evalSavingsCases = []evalAccount{
	{"FHSA", "Накопительный счёт", "CAD", true},
	{"TFSA", "Вклад Сбер", "CAD", true},
	{"RRSP", "ИИС", "CAD", true},
	{"RESP", "Брокерский счёт", "CAD", true},
	{"LIRA", "Копилка", "CAD", true},
	{"HISA", "Т-Банк Накопительный", "CAD", true},
	{"Wealthsimple TFSA", "Подушка безопасности", "CAD", true},
	{"Questrade RRSP", "Вклад ВТБ", "CAD", true},
	{"EQ Bank Savings", "Накопления на отпуск", "CAD", true},
	{"Tangerine Savings", "Инвесткопилка", "CAD", true},
	{"401k", "Долларовый вклад", "USD", true},
	{"Roth IRA", "Пенсионные накопления", "USD", true},
	{"HSA", "Сбережения", "USD", true},
	{"Ally Savings", "Тинькофф Инвестиции", "USD", true},
	{"CD 12-month", "Резервный фонд", "USD", true},
	{"Emergency Fund", "Вклад «Лучший %»", "CAD", true},
	{"Vanguard Brokerage", "БКС Брокер", "USD", true},
	{"Tagesgeld", "Накопительный Альфа", "EUR", true},
	{"Livret A", "Вклад на 6 месяцев", "EUR", true},
	{"Stocks and Shares ISA", "Копилка на машину", "GBP", true},
	{"RBC Chequing", "Сбер Дебетовая", "CAD", false},
	{"Chase Checking", "Тинькофф Black", "USD", false},
	{"Amex Gold", "Кредитная карта", "CAD", false},
	{"Visa Infinite", "Альфа Кредитка", "CAD", false},
	{"Mastercard", "Зарплатная карта ВТБ", "CAD", false},
	{"Cash", "Наличные", "CAD", false},
	{"Line of Credit", "Кредитная линия", "CAD", false},
	{"Mortgage", "Ипотека", "CAD", false},
	{"Car Loan", "Автокредит", "CAD", false},
	{"PayPal", "ЮMoney", "USD", false},
	{"Wallet", "Кошелёк", "CAD", false},
	{"Debit Card", "Карта Мир", "CAD", false},
}

type evalIconCase struct {
	en, ru, kind string // kind: "expense"/"income" for categories, the currency for accounts
	icons        []string
}

var evalCategoryIconCases = []evalIconCase{
	{"Coffee", "Кофе", "expense", []string{"local_cafe", "coffee"}},
	{"Hydro", "Электричество", "expense", []string{"bolt", "power", "electric_bolt", "lightbulb"}},
	{"Groceries", "Продукты", "expense", []string{"local_grocery_store", "shopping_cart", "shopping_basket", "trolley", "store", "shopping_bag"}},
	{"Rent", "Аренда", "expense", []string{"home", "apartment", "home_work", "house", "real_estate_agent", "cottage", "villa", "location_city"}},
	{"Restaurants", "Рестораны", "expense", []string{"restaurant", "restaurant_menu", "local_dining", "lunch_dining", "fastfood", "set_meal", "local_pizza", "room_service"}},
	{"Gas", "Бензин", "expense", []string{"local_gas_station", "ev_station", "directions_car"}},
	{"Public transit", "Общественный транспорт", "expense", []string{"directions_bus", "train", "subway", "tram", "commute", "airport_shuttle"}},
	{"Taxi", "Такси", "expense", []string{"local_taxi", "directions_car", "time_to_leave", "commute", "electric_car"}},
	{"Car insurance", "Автострахование", "expense", []string{"directions_car", "security", "health_and_safety", "car_repair", "time_to_leave", "electric_car", "umbrella", "local_police"}},
	{"Internet", "Интернет", "expense", []string{"wifi", "cloud", "computer"}},
	{"Mobile phone", "Мобильная связь", "expense", []string{"smartphone", "phone_iphone", "android"}},
	{"Streaming", "Онлайн-кинотеатры", "expense", []string{"subscriptions", "movie", "theaters", "videocam", "music_note", "local_activity"}},
	{"Gym", "Спортзал", "expense", []string{"fitness_center", "sports_gymnastics", "self_improvement", "directions_run", "sports_martial_arts"}},
	{"Pharmacy", "Аптека", "expense", []string{"local_pharmacy", "medical_services", "medical_information", "local_hospital", "health_and_safety", "emergency"}},
	{"Dentist", "Стоматология", "expense", []string{"medical_services", "local_hospital", "medical_information", "health_and_safety", "emergency", "sentiment_satisfied_alt"}},
	{"Pets", "Домашние животные", "expense", []string{"pets", "cruelty_free"}},
	{"Childcare", "Детский сад", "expense", []string{"child_care", "child_friendly", "bedroom_baby", "crib", "toys", "school"}},
	{"Tuition", "Обучение", "expense", []string{"school", "menu_book", "book", "science"}},
	{"Clothing", "Одежда", "expense", []string{"checkroom", "shopping_bag", "local_mall", "store"}},
	{"Travel", "Путешествия", "expense", []string{"flight", "flight_takeoff", "luggage", "airplane_ticket", "local_airport", "beach_access", "hotel", "backpack", "explore", "map", "nights_stay"}},
	{"Gifts", "Подарки", "expense", []string{"redeem", "celebration", "volunteer_activism", "cake", "loyalty", "local_florist", "favorite", "star"}},
	{"Charity", "Благотворительность", "expense", []string{"volunteer_activism", "favorite", "front_hand", "church", "loyalty", "support"}},
	{"Alcohol", "Алкоголь", "expense", []string{"wine_bar", "local_bar", "local_drink", "nightlife"}},
	{"Haircut", "Парикмахерская", "expense", []string{"spa", "face", "self_improvement", "mood"}},
	{"Books", "Книги", "expense", []string{"menu_book", "book", "newspaper"}},
	{"Home improvement", "Ремонт", "expense", []string{"handyman", "construction", "home_repair_service", "tools_power_drill", "carpenter", "imagesearch_roller", "house", "home_work", "chair"}},
	{"Parking", "Парковка", "expense", []string{"local_parking", "directions_car"}},
	{"Bank fees", "Банковские комиссии", "expense", []string{"account_balance", "receipt", "receipt_long", "paid", "payments", "credit_score", "currency_exchange"}},
	{"Video games", "Видеоигры", "expense", []string{"sports_esports", "smart_toy", "toys", "mouse"}},
	{"Water", "Водоснабжение", "expense", []string{"water_drop", "water", "local_drink"}},
	{"Salary", "Зарплата", "income", []string{"payments", "paid", "account_balance_wallet", "wallet", "account_balance"}},
	{"Interest", "Проценты по вкладам", "income", []string{"trending_up", "savings", "account_balance", "paid", "payments", "currency_exchange", "account_balance_wallet"}},
	{"Freelance", "Фриланс", "income", []string{"computer", "desktop_mac", "desktop_windows", "payments", "paid", "engineering", "account_balance_wallet", "mouse"}},
}

var evalAccountIconCases = []evalIconCase{
	{"Visa Infinite", "Кредитная карта Альфа", "CAD", []string{"credit_card", "credit_score", "payments", "account_balance_wallet"}},
	{"Debit Card", "Тинькофф Black", "CAD", []string{"credit_card", "account_balance", "account_balance_wallet", "payments", "wallet", "paid"}},
	{"RBC Chequing", "Сбер Дебетовая", "CAD", []string{"account_balance", "credit_card", "account_balance_wallet", "payments", "wallet", "paid"}},
	{"Wealthsimple TFSA", "ИИС", "CAD", []string{"savings", "trending_up", "bar_chart", "account_balance", "account_balance_wallet", "wallet", "currency_exchange", "paid"}},
	{"Cash", "Наличные", "CAD", []string{"wallet", "payments", "account_balance_wallet", "paid"}},
	{"Savings Account", "Накопительный счёт", "CAD", []string{"savings", "account_balance", "trending_up", "wallet", "account_balance_wallet", "paid", "payments"}},
	{"Brokerage", "Брокерский счёт", "USD", []string{"trending_up", "bar_chart", "savings", "account_balance", "account_balance_wallet", "wallet", "currency_exchange", "paid"}},
	{"Bitcoin Wallet", "Криптокошелёк", "USD", []string{"currency_bitcoin", "wallet", "account_balance_wallet", "currency_exchange", "paid"}},
	{"PayPal", "ЮMoney", "USD", []string{"account_balance_wallet", "wallet", "payments", "paid", "smartphone", "credit_card", "account_balance", "currency_exchange", "qr_code", "phone_iphone"}},
	{"Mortgage", "Ипотека", "CAD", []string{"home", "house", "apartment", "real_estate_agent", "account_balance", "cottage", "villa", "home_work", "credit_score", "payments"}},
	{"Car Loan", "Автокредит", "CAD", []string{"directions_car", "time_to_leave", "credit_score", "account_balance", "electric_car", "payments", "paid"}},
	{"Euro Account", "Евровый счёт", "EUR", []string{"euro", "euro_symbol", "account_balance", "currency_exchange", "paid", "payments", "wallet", "account_balance_wallet"}},
	{"Emergency Fund", "Подушка безопасности", "CAD", []string{"savings", "health_and_safety", "security", "emergency", "account_balance", "umbrella", "lock", "support", "wallet", "account_balance_wallet", "paid"}},
	{"Line of Credit", "Кредитная линия", "CAD", []string{"credit_score", "credit_card", "account_balance", "payments", "paid", "account_balance_wallet", "currency_exchange"}},
	{"RESP", "Копилка для детей", "CAD", []string{"savings", "child_care", "child_friendly", "school", "account_balance", "toys", "bedroom_baby"}},
	{"Mastercard", "Карта Мир", "CAD", []string{"credit_card", "credit_score", "payments", "account_balance_wallet", "account_balance"}},
}

func (c evalIconCase) name(lang string) string {
	if lang == "ru" {
		return c.ru
	}
	return c.en
}

func TestAIEval_FixturesUseCatalogueIcons(t *testing.T) {
	for _, set := range [][]evalIconCase{evalCategoryIconCases, evalAccountIconCases} {
		for _, c := range set {
			for _, icon := range c.icons {
				if !iconSet[icon] {
					t.Errorf("%s: %q is not in icons/icons.json", c.en, icon)
				}
			}
		}
	}
}

var evalUser = vo.MustParseId("019a0000-0000-7000-8000-00000000e001")

type evalAccounts struct{ rows []model.OnboardingAccount }

func (f evalAccounts) AccountsByIDs(context.Context, []vo.Id) ([]model.OnboardingAccount, error) {
	return f.rows, nil
}

type evalBudgets struct{ picked []vo.Id }

func (f *evalBudgets) MarkSavingsMembers(_ context.Context, _, _ vo.Id, ids []vo.Id) error {
	f.picked = append(f.picked, ids...)
	return nil
}

// evalIcons records the icons applyIcons writes; an item it leaves alone
// keeps its old icon.
type evalIcons struct {
	mu  sync.Mutex
	set map[string]string
}

func (f *evalIcons) record(id vo.Id, to string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set[strings.ToLower(id.String())] = to
}

type evalCategorySteps struct {
	CategorySteps
	*evalIcons
	rows []model.CategoryResult
}

func (f evalCategorySteps) OwnCategories(context.Context, vo.Id) ([]model.CategoryResult, error) {
	return f.rows, nil
}

func (f evalCategorySteps) ReplaceIcon(_ context.Context, _, id vo.Id, _, to string) (bool, error) {
	f.record(id, to)
	return true, nil
}

type evalAccountSteps struct {
	AccountSteps
	*evalIcons
	rows []model.OnboardingAccount
}

func (f evalAccountSteps) OwnLiveAccounts(context.Context, vo.Id) ([]model.OnboardingAccount, error) {
	return f.rows, nil
}

func (f evalAccountSteps) ReplaceIcon(_ context.Context, _, id vo.Id, _, to string) (bool, error) {
	f.record(id, to)
	return true, nil
}

// evalCurrency moves the Canadian cases to roubles for the Russian run.
func evalCurrency(lang, currency string) string {
	if lang == "ru" && currency == "CAD" {
		return "RUB"
	}
	return currency
}

func evalContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func TestAIEval_Savings(t *testing.T) {
	provider := aieval.Provider(t)
	defer provider.Spend(t)
	var areas []*aieval.Area
	for _, lang := range aieval.Languages {
		area := aieval.NewArea("savings/" + lang.Code)
		areas = append(areas, area)
		var rows []model.OnboardingAccount
		byID := map[string]evalAccount{}
		ids := []vo.Id{}
		for _, c := range evalSavingsCases {
			id := vo.NewId()
			name := c.en
			if lang.Code == "ru" {
				name = c.ru
			}
			rows = append(rows, model.OnboardingAccount{ID: id.String(), OwnerID: evalUser.String(), Name: name, CurrencyCode: evalCurrency(lang.Code, c.currency)})
			byID[id.String()] = c
			ids = append(ids, id)
		}
		budgets := &evalBudgets{}
		svc := &Service{accounts: evalAccounts{rows}, budgets: budgets, completer: provider}
		if err := svc.classify(evalContext(t), evalUser, vo.NewId(), ids, promptFacts{Language: lang.Code, Timezone: lang.Timezone}); err != nil {
			t.Errorf("%s: %v", area.Name, err)
			continue
		}
		picked := map[string]bool{}
		for _, id := range budgets.picked {
			picked[id.String()] = true
		}
		for _, r := range rows {
			want := byID[r.ID].savings
			area.Check(picked[r.ID] == want, r.Name, yesNo(picked[r.ID]), yesNo(want))
		}
	}
	aieval.Finish(t, areas...)
}

func yesNo(b bool) string {
	if b {
		return "savings"
	}
	return "everyday"
}

func TestAIEval_CategoryIcons(t *testing.T) {
	provider := aieval.Provider(t)
	defer provider.Spend(t)
	var areas []*aieval.Area
	for _, lang := range aieval.Languages {
		area := aieval.NewArea("category-icons/" + lang.Code)
		areas = append(areas, area)
		fake := evalCategorySteps{evalIcons: &evalIcons{set: map[string]string{}}}
		only := map[string]bool{}
		cases := map[string]evalIconCase{}
		for i, c := range evalCategoryIconCases {
			id := strings.ToLower(vo.NewId().String())
			icon := model.DefaultIcon
			if i%2 == 1 {
				icon = model.ImportCategoryIcon
			}
			fake.rows = append(fake.rows, model.CategoryResult{Id: id, Name: c.name(lang.Code), Type: c.kind, Icon: icon})
			only[id] = true
			cases[id] = c
		}
		svc := &Service{completer: provider, steps: Steps{Categories: fake}}
		if err := svc.reviewCategoryIcons(evalContext(t), evalUser, promptFacts{Language: lang.Code, Timezone: lang.Timezone}, only); err != nil {
			t.Errorf("%s: %v", area.Name, err)
			continue
		}
		for _, c := range fake.rows {
			got, ok := fake.set[c.Id]
			if !ok {
				got = c.Icon
			}
			want := cases[c.Id].icons
			area.Check(aieval.OneOf(got, want), c.Name, aieval.Show(got), aieval.ShowSet(want))
		}
	}
	aieval.Finish(t, areas...)
}

func TestAIEval_AccountIcons(t *testing.T) {
	provider := aieval.Provider(t)
	defer provider.Spend(t)
	var areas []*aieval.Area
	for _, lang := range aieval.Languages {
		area := aieval.NewArea("account-icons/" + lang.Code)
		areas = append(areas, area)
		fake := evalAccountSteps{evalIcons: &evalIcons{set: map[string]string{}}}
		only := map[string]bool{}
		cases := map[string]evalIconCase{}
		for _, c := range evalAccountIconCases {
			id := strings.ToLower(vo.NewId().String())
			fake.rows = append(fake.rows, model.OnboardingAccount{ID: id, OwnerID: evalUser.String(), Name: c.name(lang.Code), CurrencyCode: evalCurrency(lang.Code, c.kind), Icon: model.ImportAccountIcon})
			only[id] = true
			cases[id] = c
		}
		svc := &Service{completer: provider, steps: Steps{Accounts: fake}}
		if err := svc.reviewAccountIcons(evalContext(t), evalUser, promptFacts{Language: lang.Code, Timezone: lang.Timezone}, only); err != nil {
			t.Errorf("%s: %v", area.Name, err)
			continue
		}
		for _, a := range fake.rows {
			got, ok := fake.set[a.ID]
			if !ok {
				got = a.Icon
			}
			want := cases[a.ID].icons
			area.Check(aieval.OneOf(got, want), a.Name, aieval.Show(got), aieval.ShowSet(want))
		}
	}
	aieval.Finish(t, areas...)
}
