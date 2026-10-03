// Package imports is the transaction-import feature: external sources
// (push events from a phone, pull providers such as SimpleFIN), the link
// ledger that remembers every external transaction ever seen, import runs,
// and the matcher that decides whether an incoming record is a new
// transaction or one the user already entered by hand.
//
// The root knows no provider: each one lives in its own subpackage
// (applewallet, simplefin) and plugs in through the EventParser and Provider
// registries at composition time.
package imports
