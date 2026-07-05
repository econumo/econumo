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

	// CaptureIDInto, when set, is filled by Replay with the server-generated
	// entity id ("data.item.id" in this call's response) once the call
	// completes. Every CREATE endpoint treats the client-supplied "id" as an
	// idempotency operation key only and mints a fresh UUIDv7 for the entity
	// (see normalize.go's uuidV7Re doc comment) — so a later call in the same
	// scenario that must act on the just-created entity references the target
	// of this pointer (e.g. as a *string Body value, which json.Marshal
	// dereferences) rather than the client-supplied operation id.
	CaptureIDInto *string
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
