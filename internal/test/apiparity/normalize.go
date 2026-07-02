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
