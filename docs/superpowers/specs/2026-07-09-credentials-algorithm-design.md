# Credentials algorithm versioning (issue #64)

Date: 2026-07-09
Issue: https://github.com/econumo/econumo/issues/64

## Problem

Passwords are hashed with a legacy scheme (sha512, 500 iterations, per-user salt
column) that is fast to brute-force by modern standards. We need a strong
algorithm for new hashes without invalidating existing credentials.

## Decision summary

- New `users.algorithm` column records which scheme hashed the stored password.
  Values: `"sha512"` (the legacy scheme; default for all existing rows) and
  `"argon2id"` (the new scheme).
- New users are hashed with Argon2id.
- Every path that sets a new plaintext password writes an Argon2id hash and
  flips the column: update-password, reset-password, and the CLI
  `user:change-password`. Login only verifies — no rehash-on-login.
- Verification dispatches on the column value; unknown values verify as false.

## Schema

One new migration per engine (`internal/infra/storage/migrations/{sqlite,pgsql}`):

```sql
ALTER TABLE users ADD COLUMN algorithm VARCHAR(32) NOT NULL DEFAULT 'sha512';
```

Existing rows become `sha512` via the default. `algorithm` joins the column
lists of `GetUserByID`, `GetUserByIdentifier`, `InsertUser`, and `UpsertUser`
in both engines' `users.sql`; sqlc regenerated. The pgsql shim's whole-struct
conversions keep compiling because the generated structs stay field-identical.

The legacy `salt` column is untouched and still populated at registration
(NOT NULL constraint, and the sha512 verify path needs it); Argon2id embeds its
own salt in the hash string and ignores the column.

## Model (`internal/model/user.go`)

- `User.Algorithm string` field.
- Constants `AlgorithmSHA512 = "sha512"`, `AlgorithmArgon2id = "argon2id"`.
- `NewUser` stamps `AlgorithmArgon2id` (new users are always argon2id).
- `UpdatePassword(passwordHash, algorithm string, now time.Time)` sets both
  fields; all callers pass `AlgorithmArgon2id`, which is the sha512→argon2id
  transition the issue asks for.
- Repo hydrate/save round-trips the field.

## Hasher (`internal/infra/auth`)

`PasswordHasher` keeps its name and constructor; its API becomes:

- `Hash(plain string) (string, error)` — always Argon2id. OWASP parameters:
  m=19456 KiB, t=2, p=1, 16-byte random salt, 32-byte key. Output is a standard
  PHC string: `$argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64key>` (raw std
  base64, no padding). Error only on entropy-source failure.
- `Verify(algorithm, stored, plain, salt string) bool` — dispatch:
  - `"sha512"` → the existing sha512×500 path, unchanged (length-88 / `$` guard,
    constant-time compare).
  - `"argon2id"` → parse the PHC string (params come from the stored string, so
    future parameter tuning does not break old hashes), recompute, compare
    constant-time.
  - anything else → false.
- `HashSHA512(plain, salt string) string` — the legacy hash, exported for the
  dispatch path and for tests that seed legacy users.

New file `argon2id.go` holds the PHC encode/parse/verify; `password.go` keeps
the sha512 code and gains the dispatch. Dependency: `golang.org/x/crypto/argon2`
(already in the module graph; becomes a direct dependency).

## Call sites

| Site | Change |
|------|--------|
| `login.go` | `hasher.Verify(u.Algorithm, u.Password, req.Password, u.Salt)` |
| `password.go` UpdatePassword | old-password check verifies with `u.Algorithm`; new hash written as argon2id |
| `password.go` ResetPassword | new hash written as argon2id |
| `admin.go` AdminChangePassword | new hash written as argon2id |
| `register.go` createUser | hash with `Hash(plain)`; salt still generated |
| `internal/test/fixture` | seeds argon2id users (they log in through the real endpoint) |

## Wire & data contract

No API response exposes password, salt, or algorithm, so the response envelope
and apiparity goldens are expected to be byte-identical (verified, not assumed).
Login/401 behavior, validation messages, and routes are unchanged. Existing
stored hashes remain valid; ids and other columns untouched.

## Error handling

- `Hash` returns an error only if `crypto/rand` fails; callers propagate (500).
- Malformed stored PHC strings, wrong version, or unknown algorithm values fail
  verification closed (false → 401 / "Password is not correct"), never panic.

## Testing

- **auth unit + golden**: PHC round-trip, fixed-vector golden (following
  `crypto_golden_test.go`), verify-dispatch matrix (sha512 ok, argon2id ok,
  unknown alg false, malformed PHC false, wrong password false).
- **user use cases**: legacy (`sha512`) user can log in; update-password on a
  legacy user rewrites hash + `algorithm='argon2id'`; wrong old password still
  errors and leaves the row untouched; reset-password and CLI change-password
  transition likewise; register creates `argon2id`.
- **repo**: algorithm column round-trips on save/load (runs on both engines via
  the existing `make test-repo-pgsql` and enginecompare suites).
- **apiparity**: goldens unchanged; scenario/route guards unaffected (no new
  routes).

## Out of scope

- Rehash-on-login (decided against).
- Removing the `salt` column or the sha512 code (needed while any `sha512` row
  exists).
- Config-tunable Argon2id parameters.
