package onboarding

import "strings"

// Every AI prompt of the onboarding feature lives here so it can be found and
// edited in one place.

// classifySystem: classify, the savings question (save-accounts job and finish-import review).
const classifySystem = `You help set up a personal budget. A savings account holds money the user is putting aside (a savings or deposit account, an emergency fund, a piggy bank, an investment or brokerage account kept for the future); everyday accounts are for spending (cards, checking, cash, wallets). Account names may be in any language; use the user's timezone and language to read local bank and product names. Decide for each account whether it is a savings account. Reply with JSON only, exactly {"savings":["<id>", ...]} listing the ids of the savings accounts; use an empty list when none is.`

// iconRules: shared by both icon prompts; the icon list is the catalogue in icons.go.
var iconRules = `Names may be in any language; use the user's timezone and language to read local bank and product names. ` +
	`Use only icons from this list: ` + strings.Join(Icons, ", ") + `. `

// categoryIconsSystem: reviewCategoryIcons, after a CSV import.
var categoryIconsSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each category pick the icon that best shows what the money is spent on or comes from. ` + iconRules +
	`Reply with JSON only, exactly {"categories":{"<id>":"<icon>"}}; leave out a category when no icon fits.`

// accountIconsSystem: reviewAccountIcons, after a CSV import.
var accountIconsSystem = `You help set up a personal budget from the user's imported bank data. ` +
	`For each account pick the icon that best shows what kind of account it is (a card, a bank account, cash, savings, an investment). ` + iconRules +
	`Reply with JSON only, exactly {"accounts":{"<id>":"<icon>"}}; leave out an account when no icon fits.`
