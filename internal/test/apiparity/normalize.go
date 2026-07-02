package apiparity

import "regexp"

// uuidV7Re matches a version-7 UUID (the version nibble after the 2nd dash is
// '7'). Every CREATE endpoint mints a fresh server-side vo.NewId() (UUIDv7,
// time+random based) and ignores the client-supplied id (which is only the
// idempotency operation key). Those generated ids legitimately differ per run
// and per engine — they are NOT a parity property — so they are redacted before
// comparison. The fixed fixture ids (and the USD currency id) are deliberately
// NOT v7, so they survive normalization and remain strictly compared.
var uuidV7Re = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-7[0-9a-fA-F]{3}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// NormalizeParity redacts server-generated UUIDv7 ids so the comparison focuses
// on everything else (names, positions, amounts, timestamps, ordering, envelope
// shape). All other bytes are compared strictly.
func NormalizeParity(b []byte) string {
	return uuidV7Re.ReplaceAllString(string(b), "<generated-uuid>")
}

var (
	datetimeRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)
	dateRe     = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	jwtRe      = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`)

	// inviteCodeRe redacts generate-invite's freshly-minted connection code — a
	// 5-hex-char string with per-character randomized case
	// (domain/connection.GenerateConnectionCode), not a UUID, so it survives
	// NormalizeParity untouched. Anchored to the "code" JSON field so it can't
	// eat any other field (e.g. a 3-letter currency "code").
	inviteCodeRe = regexp.MustCompile(`"code":"[0-9A-Fa-f]{5}"`)
)

// NormalizeGolden makes a response body stable across runs AND engines: the
// parity redaction (UUIDv7) plus clock-derived datetimes/dates, JWTs, and the
// generate-invite response code. Everything else — field names, amounts,
// names, ordering, envelope shape, validation messages — is compared
// byte-for-byte against the golden.
func NormalizeGolden(b []byte) string {
	s := NormalizeParity(b)
	s = jwtRe.ReplaceAllString(s, "<jwt>")
	s = datetimeRe.ReplaceAllString(s, "<datetime>")
	s = dateRe.ReplaceAllString(s, "<date>")
	s = inviteCodeRe.ReplaceAllString(s, `"code":"<invite-code>"`)
	return s
}
