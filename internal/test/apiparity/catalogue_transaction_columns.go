package apiparity

import "net/url"

func init() {
	// The harness has no AI provider, so every valid suggest-import-columns
	// answers empty roles (a read: a readonly user is answered too).
	register(Scenario{Name: "transaction_suggest_import_columns", Calls: func() []Call {
		const path = "/api/v1/transaction/suggest-import-columns"
		q := func(columns string) string { return path + "?columns=" + url.QueryEscape(columns) }
		two := `[{"name":"Buchungstag","type":"date","negatives":false},{"name":"Betrag","type":"number","negatives":true}]`
		return []Call{
			{Label: "suggest-import-columns", Method: "GET", Path: q(two), Auth: "owner"},
			{Label: "suggest-import-columns-readonly", Method: "GET", Path: q(two), Auth: "readonly"},
			{Label: "err:suggest-import-columns-missing", Method: "GET", Path: path, Auth: "owner"},
			{Label: "err:suggest-import-columns-malformed", Method: "GET", Path: q(`[{"name":`), Auth: "owner"},
			{Label: "err:suggest-import-columns-blank-name", Method: "GET", Path: q(`[{"name":"  ","type":"text"}]`), Auth: "owner"},
			{Label: "err:suggest-import-columns-bad-type", Method: "GET", Path: q(`[{"name":"Betrag","type":"money"}]`), Auth: "owner"},
		}
	}})
}
