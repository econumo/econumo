//go:build upgrade

package upgradetest

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

const (
	ownerEmail     = "owner@example.test"
	partnerEmail   = "partner@example.test"
	ownerPassword  = "Upgrade-Owner-2026!"
	partnerPasswd  = "Upgrade-Partner-2026!"
	budgetMonthAug = "2026-08-01"
	budgetMonthSep = "2026-09-01"
)

// The balances the seed implies, after the upgrade and before its writes.
const (
	walletBalance = "1026.80" // 1000 - 123.45 + 250.25 - 100
	cardBalance   = "570"     // 500 + 90 - 20
)

// seeded is what the old release was given, plus what the checks need to find
// it again. Entity ids are the server's: a create call's "id" is only the
// operation id that makes a retry idempotent.
type seeded struct {
	ownerID, partnerID           string
	ownerSession, partnerSession string
	pat                          string
	usd, eur                     string
	wallet, card, cash           string
	groceries, salary            string
	payee, tag, label            string
	expense, income              string
	budget, envelope             string
	labels, recurring            bool
}

type userData struct {
	Id      string `json:"id"`
	Options []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	} `json:"options"`
}

type userEnvelope struct {
	User userData `json:"user"`
}

var opSeq int

// op returns a fresh operation id for a create call.
func op() string {
	opSeq++
	return fmt.Sprintf("0a000000-0000-4000-8000-%012d", opSeq)
}

// created reads the id of the entity a create call returned: {item: {id}} for
// most, {item: {meta: {id}}} for a budget.
func created(t *testing.T, c *client, path string, body map[string]any) string {
	t.Helper()
	var out struct {
		Item struct {
			Id   string `json:"id"`
			Meta struct {
				Id string `json:"id"`
			} `json:"meta"`
		} `json:"item"`
	}
	c.post(path, body, &out)
	if out.Item.Id != "" {
		return out.Item.Id
	}
	if out.Item.Meta.Id != "" {
		return out.Item.Meta.Id
	}
	t.Fatalf("seed: %s returned no id", path)
	return ""
}

// seed writes a small but connected household through the old release's API:
// two users in a connection, an account and a budget shared between them,
// accounts in two currencies, a transfer between them, classified
// transactions, budget limits and an envelope, a personal access token, and an
// account deleted with a non-zero balance. Labels and recurring templates are
// added on releases that have them.
func seed(t *testing.T, in *instance, from semver) seeded {
	t.Helper()
	anon := in.client(t, "")
	var s seeded

	anon.post("/api/v1/user/register-user", map[string]any{"email": ownerEmail, "password": ownerPassword, "name": "Upgrade Owner"}, nil)
	anon.post("/api/v1/user/register-user", map[string]any{"email": partnerEmail, "password": partnerPasswd, "name": "Upgrade Partner"}, nil)
	ownerLogin := anon.login(ownerEmail, ownerPassword)
	partnerLogin := anon.login(partnerEmail, partnerPasswd)
	s.ownerID, s.ownerSession = ownerLogin.User.Id, ownerLogin.Token
	s.partnerID, s.partnerSession = partnerLogin.User.Id, partnerLogin.Token
	owner := in.client(t, s.ownerSession)
	partner := in.client(t, s.partnerSession)

	var me userEnvelope
	owner.get("/api/v1/user/get-user-data", &me)
	for _, o := range me.User.Options {
		if o.Name == "currency_id" {
			s.usd, _ = o.Value.(string)
		}
	}
	var currencies struct {
		Items []struct{ Id, Code string } `json:"items"`
	}
	owner.get("/api/v1/currency/get-currency-list", &currencies)
	for _, c := range currencies.Items {
		if c.Code == "EUR" {
			s.eur = c.Id
		}
	}
	if s.usd == "" || s.eur == "" {
		t.Fatalf("seed: USD %q / EUR %q not found", s.usd, s.eur)
	}

	folder := createFolder(t, owner, "Everyday")
	account := func(name, currency, balance, icon string) string {
		return created(t, owner, "/api/v1/account/create-account", map[string]any{
			"id": op(), "name": name, "currencyId": currency, "balance": balance, "icon": icon, "folderId": folder,
		})
	}
	s.wallet = account("Wallet", s.usd, "1000.00", "wallet")
	s.card = account("Euro card", s.eur, "500", "credit_card")
	s.cash = account("Old cash", s.usd, "75.00", "payments")
	s.groceries = created(t, owner, "/api/v1/category/create-category", map[string]any{"id": op(), "name": "Groceries", "type": "expense", "icon": "shopping_cart"})
	s.salary = created(t, owner, "/api/v1/category/create-category", map[string]any{"id": op(), "name": "Salary", "type": "income", "icon": "work"})
	s.payee = created(t, owner, "/api/v1/payee/create-payee", map[string]any{"id": op(), "name": "Corner market"})
	s.tag = created(t, owner, "/api/v1/tag/create-tag", map[string]any{"id": op(), "name": "trip"})
	var labelIDs []string
	if r := owner.do("POST", "/api/v1/label/create-label", map[string]any{"id": op(), "name": "Business"}); r.status != 404 {
		s.label = itemID(t, r)
		s.labels = true
		labelIDs = []string{s.label}
	}

	tx := func(typ, amount, account, date, description string, extra map[string]any) string {
		body := map[string]any{"id": op(), "type": typ, "amount": amount, "accountId": account, "date": date, "description": description}
		for k, v := range extra {
			body[k] = v
		}
		return created(t, owner, "/api/v1/transaction/create-transaction", body)
	}
	s.expense = tx("expense", "123.45", s.wallet, "2026-08-15 10:30:00", "Weekly groceries",
		map[string]any{"categoryId": s.groceries, "payeeId": s.payee, "tagId": s.tag, "labelIds": labelIDs})
	s.income = tx("income", "250.25", s.wallet, "2026-08-16 09:15:00", "Bonus", map[string]any{"categoryId": s.salary})
	tx("transfer", "100", s.wallet, "2026-08-17 12:00:00", "To the euro card",
		map[string]any{"accountRecipientId": s.card, "amountRecipient": "90"})
	tx("expense", "5.00", s.cash, "2026-08-20 08:00:00", "Coffee", map[string]any{"categoryId": s.groceries})
	tx("expense", "20", s.card, "2026-09-03 18:45:00", "Groceries abroad",
		map[string]any{"categoryId": s.groceries, "payeeId": s.payee})

	s.recurring = owner.tryPost("/api/v1/recurring/create-recurring-transaction", map[string]any{
		"id": op(), "type": "expense", "amount": "50.00", "accountId": s.wallet, "categoryId": s.groceries,
		"schedule": "monthly", "nextPaymentAt": "2027-01-01 00:00:00", "description": "Delivery box",
	}, nil)

	// excludedAccounts is the pre-v1.4 shape, accountIds the current one; each
	// release ignores the field it does not know.
	budgetCreated := time.Now()
	s.budget = created(t, owner, "/api/v1/budget/create-budget", map[string]any{
		"id": op(), "name": "Household", "startDate": budgetMonthAug, "currencyId": s.usd,
		"excludedAccounts": []string{}, "accountIds": []string{s.wallet, s.card, s.cash},
	})
	owner.post("/api/v1/budget/set-limit", map[string]any{"budgetId": s.budget, "elementId": s.groceries, "period": budgetMonthAug, "amount": "400.00"}, nil)
	s.envelope = created(t, owner, "/api/v1/budget/create-envelope", map[string]any{
		"budgetId": s.budget, "id": op(), "name": "Holidays", "icon": "beach_access", "currencyId": s.usd,
		"categories": []string{}, "side": "expense",
	})
	owner.post("/api/v1/budget/set-limit", map[string]any{"budgetId": s.budget, "elementId": s.envelope, "period": budgetMonthSep, "amount": "150"}, nil)

	var invite struct {
		Item struct {
			Code string `json:"code"`
		} `json:"item"`
	}
	owner.post("/api/v1/connection/generate-invite", map[string]any{}, &invite)
	partner.post("/api/v1/connection/accept-invite", map[string]any{"code": invite.Item.Code}, nil)
	owner.post("/api/v1/account/grant-access", map[string]any{"accountId": s.wallet, "userId": s.partnerID, "role": "user"}, nil)
	partner.post("/api/v1/account/accept-access", map[string]any{"accountId": s.wallet, "folderId": createFolder(t, partner, "Shared")}, nil)
	owner.post("/api/v1/budget/grant-access", map[string]any{"budgetId": s.budget, "userId": s.partnerID, "role": "user"}, nil)
	partner.post("/api/v1/budget/accept-access", map[string]any{"budgetId": s.budget}, nil)

	var pat struct {
		Token string `json:"token"`
	}
	owner.post("/api/v1/user/create-personal-token", map[string]any{"name": "Upgrade script", "scope": "full", "expiresAt": ""}, &pat)
	if pat.Token == "" {
		t.Fatal("seed: create-personal-token returned no token")
	}
	s.pat = pat.Token

	// Releases before v1.4.0 had no explicit budget membership; its migration
	// seeds a deleted account only when it was deleted after the budget was
	// created, compared at one-second precision (PostgreSQL rounds to it).
	if from.less(semver{1, 4, 0}) {
		time.Sleep(time.Until(budgetCreated.Add(2 * time.Second)))
	}
	owner.post("/api/v1/account/delete-account", map[string]any{"id": s.cash}, nil)
	return s
}

func itemID(t *testing.T, r response) string {
	t.Helper()
	var out struct {
		Item struct{ Id string } `json:"item"`
	}
	if r.status != 200 || json.Unmarshal(r.data, &out) != nil || out.Item.Id == "" {
		t.Fatalf("seed: HTTP %d: %s", r.status, r.raw)
	}
	return out.Item.Id
}

func createFolder(t *testing.T, c *client, name string) string {
	t.Helper()
	var folder struct {
		Item struct{ Id string } `json:"item"`
	}
	c.post("/api/v1/account/create-folder", map[string]any{"name": name}, &folder)
	if folder.Item.Id == "" {
		t.Fatal("seed: create-folder returned no id")
	}
	return folder.Item.Id
}
