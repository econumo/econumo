package imports

import (
	"encoding/json"

	"github.com/econumo/econumo/internal/infra/ai"
)

// The system prompts live in the top-level prompts package.

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
