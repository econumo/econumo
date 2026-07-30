package api

import "testing"

// TestBoolQuery pins strconv.ParseBool semantics for the "uncategorized" query
// param: swagger declares it as a boolean, so a generated client may send any
// spelling ParseBool accepts (e.g. "True"), not just the literal "true"/"1".
// A stricter parse would silently fall through to a different query branch
// instead of erroring -- see the get-transaction-list handler.
func TestBoolQuery(t *testing.T) {
	cases := map[string]bool{
		"true":    true,
		"True":    true,
		"1":       true,
		"t":       true,
		"TRUE":    true,
		"false":   false,
		"False":   false,
		"0":       false,
		"":        false,
		"garbage": false,
	}
	for input, want := range cases {
		if got := boolQuery(input); got != want {
			t.Errorf("boolQuery(%q) = %v, want %v", input, got, want)
		}
	}
}
