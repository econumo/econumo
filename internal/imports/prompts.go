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
For each row decide which part of the row's text identifies the merchant, the merchant's common name, and, only when you are highly confident, which category it belongs to.
Rules:
- Include "categoryId" only when highly confident, and only an id from the given list, which holds only categories of the rows' type. Never invent ids.
- "matchText" is the shortest substring of "payee" that identifies the merchant, copied exactly as written.
- "merchant" is the merchant's common name, for example "Amazon" for "AMZN MKTP US*2K4". Use null when you cannot tell.
- "key" is the row's key, copied exactly.
- Names may be in any language; use the timezone and language to read local merchant names.
- Use null for fields you are not confident about, and leave out whole rows you are not confident about.
Reply with JSON only: {"rows":[{"key":"<key>","matchText":"...","merchant":"..." or null,"categoryId":"..." or null}]}`

// payeeSystemPrompt: suggestion step 2 (askSuggestions), picks a payee among
// each row's candidates. Reply: payeeSchema.
const payeeSystemPrompt = `You match imported bank and card transactions to payees a user already has in a personal finance app.
Each row lists the transaction's payee text, the merchant's name, and a few candidate payees.
Rules:
- Pick a payee only when it is clearly the same merchant, and only an id from that row's candidates. Never invent ids.
- "key" is the row's key, copied exactly.
- Use null for "payeeId", or leave the row out, when no candidate is clearly the same merchant.
Reply with JSON only: {"rows":[{"key":"<key>","payeeId":"..." or null}]}`

// suggestMatchTextSystem: SuggestMatchText, the rule editor's AI match text.
// Reply: matchTextSchema.
const suggestMatchTextSystem = `You help write a bank-import rule. Given one imported transaction as JSON, reply with JSON only: {"matchText":"..."}, or {"matchText":null} when no part of the payee identifies a merchant.
matchText is the shortest substring of the payee that identifies the merchant, copied exactly from the text, with no store numbers, dates or reference codes.
Copy the merchant part character for character as it appears in the text; never translate, expand or correct it. The language and timezone only help you recognise local merchant names and date formats.`

// The reply schemas follow the providers' structured-output subset: every
// object closed with all its properties required, optional values nullable.
var (
	suggestSchema = &ai.Schema{Name: "import_suggestions", JSON: json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","properties":{` +
		`"key":{"type":"string"},"matchText":{"type":"string"},"merchant":{"type":["string","null"]},"categoryId":{"type":["string","null"]}},` +
		`"required":["key","matchText","merchant","categoryId"],"additionalProperties":false}}},"required":["rows"],"additionalProperties":false}`)}

	payeeSchema = &ai.Schema{Name: "import_payees", JSON: json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","properties":{` +
		`"key":{"type":"string"},"payeeId":{"type":["string","null"]}},` +
		`"required":["key","payeeId"],"additionalProperties":false}}},"required":["rows"],"additionalProperties":false}`)}

	matchTextSchema = &ai.Schema{Name: "import_match_text", JSON: json.RawMessage(`{"type":"object","properties":{"matchText":{"type":["string","null"]}},` +
		`"required":["matchText"],"additionalProperties":false}`)}
)
