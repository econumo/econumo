// Package apiparity holds the shared API scenario catalogue: an ordered set of
// HTTP call sequences replayed against the REAL production handler
// (server.BuildAPI). Two consumers: the untagged sqlite smoke suite in this
// package (golden files, every `make test`) and the build-tagged enginecompare
// parity suite (sqlite-vs-pgsql byte equality, `make regression`).
package apiparity

// Call is one request in a scenario plus a label for diff messages.
type Call struct {
	Label string // unique within a scenario; prefix "err:" marks an expected-non-2xx call

	Method string // "GET" | "POST"
	Path   string // "/api/v1/…", MAY carry a query string

	// Auth selects which seeded user's token to attach: "owner", "guest", or ""
	// (public/no token).
	Auth string

	Body any // JSON-marshalled when non-nil

	// For non-JSON requests (multipart import). When RawBody != nil it wins over
	// Body.
	RawBody     []byte
	ContentType string
}

// Scenario returns the ordered list of calls to replay. Calls is a func (not a
// static slice) so a scenario can build a body referencing freshly-generated ids
// if needed; most just return a literal list.
type Scenario struct {
	Name  string
	Calls func() []Call
}

var catalogue []Scenario

// register adds a scenario to the shared catalogue. Each scenario file calls it
// from its own init(), so the registration order matches source order across
// the package's files.
func register(s Scenario) { catalogue = append(catalogue, s) }

// Catalogue returns every registered scenario, in registration order.
func Catalogue() []Scenario { return catalogue }
