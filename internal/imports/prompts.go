package imports

import (
	"encoding/json"

	"github.com/econumo/econumo/internal/infra/ai"
)

// Every AI prompt of the imports feature lives here so it can be found and
// edited in one place.

// suggestSystemPrompt: suggestion step 1 (askSuggestions), one request per
// transaction type. Reply: suggestSchema.
const suggestSystemPrompt = `You help classify imported bank and card transactions for a personal finance app.
For each row decide which part of the row's text identifies the merchant, the names the merchant is known by, and, when you are confident, which category it belongs to.
Rules:
- "matchText" is the shortest substring of "payee" that identifies the merchant, copied exactly as written.
- "merchants" lists up to 4 names of the merchant, the most common first, for example ["Amazon"] for "AMZN MKTP US*2K4" or ["Yandex Taxi","Яндекс Такси","Yandex Go"] for "YANDEX*TAXI". When the merchant's name is commonly written differently in the request's language or script, include that form too; add a well-known short or brand form when there is one. Use an empty list when you cannot tell.
- "categoryKey" is a key from the given category list, which holds only categories of the rows' type, copied exactly. Never invent keys.
- Be confident about a category when the merchant is a well-known chain or brand (a supermarket, a coffee chain, an online store, a fuel station chain, a streaming service) or when its name clearly says what it sells (a farmers market, a pharmacy, a gas station, a cinema), and the list has a category that fits it.
- Use null for "categoryKey" when no listed category fits, when the merchant is unknown or ambiguous, and for person-to-person transfers, generic payments or payment processors that name no merchant.
- "key" is the row's key, copied exactly.
- Names may be in any language; use the timezone and language to read local merchant names.
- Leave out whole rows you cannot read at all.
Reply with JSON only: {"rows":[{"key":"<key>","matchText":"...","merchants":["..."],"categoryKey":"..." or null}]}`

// payeeSystemPrompt: suggestion step 2 (askSuggestions), picks a payee among
// each row's candidates. Reply: payeeSchema.
const payeeSystemPrompt = `You match imported bank and card transactions to payees a user already has in a personal finance app.
Each row lists the transaction's payee text, the names the merchant is known by, and a few candidate payees.
Rules:
- Pick a payee only when it is clearly the same merchant, and only a key from that row's candidates, copied exactly. Never invent keys.
- "key" is the row's key, copied exactly.
- Use null for "payeeKey", or leave the row out, when no candidate is clearly the same merchant.
Reply with JSON only: {"rows":[{"key":"<key>","payeeKey":"..." or null}]}`

// suggestMatchTextSystem: SuggestMatchText, the rule editor's AI match text.
// Reply: matchTextSchema.
const suggestMatchTextSystem = `You help write a bank-import rule. Given one imported transaction as JSON, reply with JSON only: {"matchText":"..."}, or {"matchText":null} when no part of the payee identifies a merchant.
matchText is the shortest substring of the payee that identifies the merchant, copied exactly from the text, with no store numbers, dates or reference codes.
Copy the merchant part character for character as it appears in the text; never translate, expand or correct it. The language and timezone only help you recognise local merchant names and date formats.`

// The reply schemas follow the providers' structured-output subset: every
// object closed with all its properties required, optional values nullable.
var (
	suggestSchema = &ai.Schema{Name: "import_suggestions", JSON: json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","properties":{` +
		`"key":{"type":"string"},"matchText":{"type":"string"},"merchants":{"type":"array","items":{"type":"string"}},"categoryKey":{"type":["string","null"]}},` +
		`"required":["key","matchText","merchants","categoryKey"],"additionalProperties":false}}},"required":["rows"],"additionalProperties":false}`)}

	payeeSchema = &ai.Schema{Name: "import_payees", JSON: json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","properties":{` +
		`"key":{"type":"string"},"payeeKey":{"type":["string","null"]}},` +
		`"required":["key","payeeKey"],"additionalProperties":false}}},"required":["rows"],"additionalProperties":false}`)}

	matchTextSchema = &ai.Schema{Name: "import_match_text", JSON: json.RawMessage(`{"type":"object","properties":{"matchText":{"type":["string","null"]}},` +
		`"required":["matchText"],"additionalProperties":false}`)}
)
