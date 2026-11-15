package onboarding

import (
	"encoding/json"

	"github.com/econumo/econumo/icons"
	"github.com/econumo/econumo/internal/infra/ai"
)

// The system prompts live in the top-level prompts package.

// The reply schemas follow the providers' structured-output subset: every
// object closed with all its properties required.
var (
	savingsSchema = &ai.Schema{Name: "savings_accounts", JSON: json.RawMessage(`{"type":"object","properties":{"savings":{"type":"array","items":{"type":"string"}}},` +
		`"required":["savings"],"additionalProperties":false}`)}

	categoryIconsSchema = iconsSchema("category_icons", "categories")
	accountIconsSchema  = iconsSchema("account_icons", "accounts")

	debtsSchema = &ai.Schema{Name: "loan_categories", JSON: json.RawMessage(`{"type":"object","properties":{"debts":{"type":"array","items":{"type":"object",` +
		`"properties":{"categoryKey":{"type":"string"}},"required":["categoryKey"],"additionalProperties":false}}},"required":["debts"],"additionalProperties":false}`)}

	suggestIconSchema = func() *ai.Schema {
		enum, _ := json.Marshal(append([]string{""}, icons.Names...))
		return &ai.Schema{Name: "icon_suggestion", JSON: json.RawMessage(`{"type":"object","properties":{"icon":{"type":"string","enum":` + string(enum) + `}},` +
			`"required":["icon"],"additionalProperties":false}`)}
	}()
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
