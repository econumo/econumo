// Package model is the application's shared type universe: every feature's
// entities (with their invariant-preserving mutators), value objects, and
// request/result DTOs live here, named per feature file (account.go,
// account_dto.go, ...). Behavior stays in the feature packages — they import
// model; model imports only the shared kernel. One definition per concept:
// cross-feature reads use these types directly instead of structural copies.
package model
