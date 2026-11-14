package onboarding

import (
	"encoding/json"
	"strings"

	"github.com/econumo/econumo/icons"
	"github.com/econumo/econumo/internal/infra/ai"
)

// Every AI prompt of the onboarding feature lives here so it can be found and
// edited in one place.

// classifySystem: classify, the savings question (save-accounts job and
// finish-import review). Reply: savingsSchema.
const classifySystem = `You help set up a personal budget. A savings account holds money the user is putting aside (a savings or deposit account, an emergency fund, a piggy bank, an investment or brokerage account kept for the future); everyday accounts are for spending (cards, checking, cash, wallets). Account names may be in any language; use the user's timezone and language to read local bank and product names. Decide for each account whether it is a savings account. Each account has a short "key". Reply with JSON only, exactly {"savings":["<key>", ...]} listing the keys of the savings accounts, copied exactly; use an empty list when none is.`

// iconRules: shared by both icon prompts; the icon list is the shared catalogue icons/icons.json.
var iconRules = `Names may be in any language; use the user's timezone and language to read local bank and product names. ` +
	`Use only icons from this list: ` + strings.Join(icons.Names, ", ") + `. ` +
	`Each item has a short "key"; copy it exactly. `

// categoryIconsSystem: reviewCategoryIcons, after a CSV import. Reply:
// categoryIconsSchema.
var categoryIconsSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each category pick the icon that best shows what the money is spent on or comes from. ` + iconRules +
	`Reply with JSON only, exactly {"categories":[{"key":"<key>","icon":"<icon>"}]}; leave out a category when no icon fits.`

// accountIconsSystem: reviewAccountIcons, after a CSV import. Reply:
// accountIconsSchema.
var accountIconsSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each account pick the icon that best shows what kind of account it is (a card, a bank account, cash, savings, an investment). ` + iconRules +
	`Reply with JSON only, exactly {"accounts":[{"key":"<key>","icon":"<icon>"}]}; leave out an account when no icon fits.`

// The reply schemas follow the providers' structured-output subset: every
// object closed with all its properties required.
var (
	savingsSchema = &ai.Schema{Name: "savings_accounts", JSON: json.RawMessage(`{"type":"object","properties":{"savings":{"type":"array","items":{"type":"string"}}},` +
		`"required":["savings"],"additionalProperties":false}`)}

	categoryIconsSchema = iconsSchema("category_icons", "categories")
	accountIconsSchema  = iconsSchema("account_icons", "accounts")
)

// iconsSchema is {list:[{key, icon}]}, icon limited to the catalogue.
func iconsSchema(name, list string) *ai.Schema {
	enum, _ := json.Marshal(icons.Names)
	return &ai.Schema{Name: name, JSON: json.RawMessage(`{"type":"object","properties":{"` + list + `":{"type":"array","items":{"type":"object","properties":{` +
		`"key":{"type":"string"},"icon":{"type":"string","enum":` + string(enum) + `}},` +
		`"required":["key","icon"],"additionalProperties":false}}},"required":["` + list + `"],"additionalProperties":false}`)}
}

var iconSet = func() map[string]bool {
	m := make(map[string]bool, len(icons.Names))
	for _, i := range icons.Names {
		m[i] = true
	}
	return m
}()
