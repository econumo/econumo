# Identified product analytics

Twillingate analytics move from anonymous to identified: every event carries a
hashed user id, a per-instance host, and a small profile of the account. The
instance-wide `ECONUMO_ANALYTICS` switch is replaced by a per-user preference
stored in `users_options` and exposed in Settings.

## 1. Why

Anonymous mode salts and daily-rotates every identifier, so nothing links across
days: no retention curves, no "how many users does this instance have", no
per-user funnels. The collector supports an `identified` mode that stores
`$user_id` as given; this design adopts it while keeping raw identifiers —
account ids, self-hosted hostnames, user names — out of the analytics store.
Cloud hostnames are sent verbatim, as they always have been.

## 2. Privacy position

No consent banner. Under ePrivacy Art. 5(3) a banner is required for storing or
reading data on the user's device; the transport writes nothing (`$install_id`
is an in-memory per-page-load UUID) and the user id arrives from the API
response of an already-authenticated session. **This design must not adopt the
Twillingate JS SDK**, whose `identified` mode persists a visitor id in
`localStorage` — the collector's own docs class that as consent-relevant.

Under GDPR the events become personal data, so the design ships three things
together: disclosure (privacy-policy revision), a lawful basis (legitimate
interest), and a working right to object (the Settings toggle). The
localStorage mirror of the opt-out (§5.4) is a user-preference store, the
ePrivacy-exempt category, and does not disturb the above.

`$user_name` is never sent. In identified mode the collector stores it
verbatim; the hashed id alone serves every question we want to ask.

## 3. Identity

### 3.1 User

```
$user_id = sha256("econumo:analytics:v1:" + user.id).hex[:32]
```

Sent as a batch-level attribute. The hash means the analytics store never holds
a value that is also a primary key in the application database, so a leak on
that side does not hand over app user ids. It is deliberately **not**
unlinkable by us: holding the user table we can hash and join, which is what
keeps support lookups possible.

Computed in `web/src/lib/analyticsId.ts` with a self-contained synchronous
SHA-256 (~40 lines, no dependency). **Not `crypto.subtle`**: it is
secure-context-only and therefore missing on a self-hosted instance served over
plain `http://<lan-ip>`, the same landmine that already forced `uuid.v4` over
`crypto.randomUUID` in `analytics.ts`. The unit test cross-checks the
implementation against Node's webcrypto digest.

### 3.2 Instance

`schema_migrations(version, applied_at)` is created on the first boot of the Go
binary on every deployment and both engines; `recordVersion` stamps
`nowUTC()` for every version, including the ones adopted from the legacy
`migration_versions` table, and never rewrites a row.

```
anchor   = SELECT version, applied_at FROM schema_migrations
           ORDER BY applied_at ASC, version ASC LIMIT 1
instance = sha256("econumo:instance:v1:" + version + "|" + applied_at).hex[:12]
```

`applied_at` is normalized through the frozen `datetime` layout (second
precision) before hashing, so the two drivers' timestamp representations cannot
produce different digests for the same instance.

The same digest identifies the instance as a Twillingate **group**: every batch
carries `$group_id` equal to the bare 12-hex value, for cloud and self-hosted
alike — never the host string. Per-instance activity and per-instance user
counts then come from the collector's own group surfaces rather than ad-hoc
SQL. Cloud and demo are separate databases and therefore separate groups.

`$group_name` carries the `host` value of §4, so the collector's group surfaces
read `app.econumo.com`, `demo.econumo.com`, `selfhosted_a3f19c02b7d4` instead
of bare digests. This discloses no real hostname: `host` is already synthetic
for everything outside `*.econumo.com`. `$group_name` is persisted in both
identity modes.

The self-hosted `host` embeds the same digest (`selfhosted_<group>`), so the
two dimensions join trivially. `$group_id` is stored raw in both identity modes
by design; note that a single-user self-hosted instance is a single-person
group and should be treated as personal data.

Resolved once in `server.BuildAPI` and merged into the served
`econumo-config.js` as `INSTANCE_ID`, preserving `spa.Handler`'s "overrides are
fixed for the process lifetime" property. Added to `MERGED_KEYS` in
`web/src/lib/appConfig.ts`, the mobile app's config allowlist, or app-mode
events against a self-hosted backend would carry no instance.

Rejected alternatives, with the reason each failed:

| Anchor | Why not |
| --- | --- |
| `app_settings` table | Correct, but a new table plus a boot write for one value. |
| A file with a random string | `docker-compose.yml` mounts only `/app/var/db`, so a file elsewhere dies on container recreation; `ProtectSystem=strict` and volume-less k8s break it too. Fails silently by regenerating, inflating instance counts with phantoms. |
| First `users.id` | Stable and high-entropy, but absent until the first registration, so fresh installs are invisible until a restart. |
| `sha256(hostname)` | Hostnames are enumerable (certificate transparency, DNS datasets), so the digest reverses by dictionary. Obfuscation, not anonymisation. |
| Seeded currency ids | The migration hardcodes them (`dffc2a06-…` for USD); identical on every instance. |

Accepted costs: uniqueness is at install-second granularity, so two instances
first booting in the same second collide (well under one expected pair across
thousands of installs); and recreating the database resets the identity, so a
`data:import-sqlite` migration from SQLite to PostgreSQL reads as a new
instance.

### 3.3 Logout

`LogoutPage` clears the user id **after** `trackEvent(METRICS.USER_LOGOUT)` and
before `removeToken()`, so the logout event still carries the identity, and
mints a fresh `$install_id` so the next person on a shared browser is not
linked to the previous one.

## 4. Event attributes

`host` is the only existing attribute whose meaning changes:

- `econumo.com` and `*.econumo.com` — verbatim, as today.
- everything else — `selfhosted_<instance>` (§3.2), e.g.
  `selfhosted_a3f19c02b7d4`.

`current_url` follows, so a self-hosted page reads
`https://selfhosted_a3f19c02b7d4/budget`. Both derive from one
`isCloudHost(hostname)` predicate, since the old check compared against the
literal `'self-hosted'` string that no longer exists.

### 4.1 Versions

The custom `version` attribute is replaced by two reserved keys, which the
collector rolls up automatically and which therefore must not be declared:

- `$app_version` — the client build, `getVersion()`.
- `$platform` — `web`, `ios` or `android`.

On the web these report the server's version too, because the SPA is embedded
in the binary and the merged `VERSION` is server truth. In the mobile app
`VERSION` is deliberately absent from the `MERGED_KEYS` allowlist, so
`$app_version` is the app build and the server's version is not reported;
an instance used exclusively through the app therefore has no version signal.
Accepted: web users cover the version question for every instance that has any.

The build plumbing is already correct and needs no change: `mobile-testflight`
exports `ECONUMO_VERSION = v$(APP_VERSION)`, and the Docker build passes the
release version, so only local dev builds report `dev`.

Dropping the custom `version` splits history the same way `deployment` does.

### 4.2 Deployment and profile

`self_hosted` (boolean) is **renamed** to `deployment` (`cloud` |
`self-hosted`). History splits at the switch: older rows carry the boolean,
newer ones the string, and a query spanning both must coalesce. Existing
`self_hosted` rollups stay intact in the aggregate tables.

Nine profile counts are added as **batch-level** attributes (sent once per
flush; the collector merges batch defaults into every event key by key):

| Attribute | Source |
| --- | --- |
| `connections` | `useConnections()` |
| `accounts`, `accounts_hidden` | `useAccounts()` + `useFolders()` |
| `categories`, `categories_archived` | category list |
| `payees`, `payees_archived` | payee list |
| `tags`, `tags_archived` | tag list |

`AccountDto` carries no visibility flag — only `FolderDto.isVisible` — so
`accounts_hidden` counts accounts in hidden folders; unfoldered accounts count
as visible.

They are read from the TanStack query cache at capture time, in
`web/src/lib/analyticsProfile.ts`, and a count that has not loaded is omitted
rather than sent as zero. Sourcing them server-side was rejected: the
dependency rule forbids `user` importing `account`/`connection`/`category`/
`payee`/`tag`, so it would need five consumer-side interfaces, five glue
adapters, per-engine COUNT queries, and six extra counts on every
`get-user-data`.

Counts are sent raw, not bucketed — SQL can bucket later, but cannot unbucket.

Registration date ships as a stable cohort pair, `signup_year` (e.g. `2026`)
and `signup_month` (`1`-`12`), read in UTC. The pair is preferred over a single
"months old" value because a cohort label never drifts, while an age attribute
changes for every user every month; age stays derivable in SQL from the event
date. This is the section's one backend change: `CurrentUserResult` gains
`createdAt` in the frozen `2006-01-02 15:04:05` layout (the entity already
carries `CreatedAt`).

The project's declared attributes become: `access_state`, `host`, `locale`,
`mode`, `deployment`, `signup_year`, `signup_month`,
`connections`, `accounts`, `accounts_hidden`, `categories`,
`categories_archived`, `payees`, `payees_archived`, `tags`, `tags_archived`.
`version` is dropped; `$app_version` and `$platform` are reserved keys and must
**not** be declared (the collector ignores `$`-prefixed entries in the list).

## 5. The opt-out preference

### 5.1 Storage

A fifth row in `users_options`, alongside `currency`, `report_period`,
`budget` and `onboarding`. `toCurrentUserWithEmail` already serializes options
generically into `options: [{name, value}]`, so **there is no schema migration
and no new DTO field** — it rides the existing array like `currency_id` does.

- name `analytics`, value `"1"` / `"0"`, matching the frozen 0/1 boolean
  convention; absent reads as enabled.
- `User.AnalyticsEnabled()` reads it; `User.SetAnalytics(enabled, id, now)`
  writes it, **appending the option when the row is missing** (`Repo.Save`
  upserts every entry in `u.Options`), so users predating the option need no
  special handling.
- Registration calls `SetAnalytics(analyticsDefault, nextID(), now)` after
  `SeedDefaultOptions`, so one code path creates the row.

### 5.2 Endpoint

`POST /api/v1/user/update-analytics`, body `{"enabled": bool}`, returning the
refreshed user — a direct analogue of `update-report-period` through
`Service.mutate`. Handler in `internal/user/api/analytics.go`, registered in
`routes.go`. No MCP tool: `internal/user/mcp` exposes only `get_user`.

**It goes in the read-only allowlist** in `internal/web/middleware/auth.go`.
Every other POST 402s a `readonly` caller, and a user whose trial lapsed is
exactly someone who may want to withdraw from analytics; an objection that only
works while you are paying is not an objection. It sits beside the existing
account-security exemptions, with a middleware test asserting the 200.

### 5.3 UI

A `privacy` group on Settings → Profile, after `preferences`, holding one
`Switch` row bound to a `useUpdateAnalytics` mutation. `METRICS.USER_UPDATE_ANALYTICS`
(`appUserUpdateAnalytics`, `{enabled}`) fires **before** the opt-out takes
effect and **after** an opt-in, so the last event of an opting-out user still
lands and `metrics-coverage.test.ts` stays green.

### 5.4 Client-side gate

`get-user-data` resolves after the first pageview fires, so a purely
server-sourced flag would still leak one event per page load. The preference is
mirrored to localStorage on every user-data read and consulted synchronously at
boot; the server stays the source of truth.

The mirror is **not** cleared on logout — it is a device-level fail-safe so an
opted-out user's login-page events do not resume the moment they sign out, and
it is corrected as soon as the next user's data loads. Consequences:

- logged out — the mirror if present, otherwise enabled (the new default);
- logged in — the user's option, mirrored on arrival.

The gate suppresses both the collector capture and the `window.dataLayer` push:
liltag/GTM tags are third-party, so an objection must stop them too.

## 6. Removing ECONUMO_ANALYTICS

The variable stops reaching the application: it is removed from the
`econumo-config.js` merge in `server.BuildAPI`, from `analyticsEnabled()` and
`EconumoConfig` in `web/src/lib/config.ts`, from every call site in
`metrics.ts`, and from `MERGED_KEYS` in `appConfig.ts` (replaced there by
`INSTANCE_ID`). Analytics default to on.

`config.Config.Analytics` **stays**, with its current strict parse, marked
deprecated and consumed by exactly one code path:
`migration:seed-analytics-option` (§7). This mirrors `ECONUMO_DATA_SALT`, which
is likewise ignored by the API and read only by `data:remove-salt` via
`c.cfg.DataSalt`, and is documented next to it in CLAUDE.md. `.env.example`
drops the entry.

The migration's one-time read is what makes the removal safe: without it, every
self-hoster who set `ECONUMO_ANALYTICS=false` would silently resume sending
events on upgrade, since users default to on.

Accepted consequence: after this, a self-hosted operator has no instance-level
control. Their users can each opt out; the operator cannot decide for the
deployment except by blocking the collector at the network.

## 7. Backfill migration

`migrations.RegisterCommand("20260903000000", "migration:seed-analytics-option")`,
a command step with no `.sql` file, in the shape of `migration:zero-deleted-accounts`.
A SQL migration cannot do this job: it cannot read the environment, and it
cannot mint UUIDv7 ids (SQLite has no UUID function).

The command reads `c.cfg.Analytics` (§6), selects users with
no `analytics` option row, and inserts one each with a fresh UUIDv7 valued
`"0"` when the variable says false and `"1"` otherwise, in one transaction,
printing the count. Idempotent through `WHERE NOT EXISTS`. It lives in
`internal/cli/migration_commands.go` over a new `user.Service` method, mirroring
`account.ZeroDeletedAccounts`.

The runner records the version once, so flipping the variable afterwards does
nothing — by then the variable no longer exists.

## 8. Collector configuration

`update_project app.econumo.com identity=identified`, plus the declared
attribute list from §4. Switching is privacy-significant and not retroactive:
existing history stays daily-hashed and retention curves start from the switch
day. Logged-out traffic keeps the random per-page-load `$install_id`, so
pre-auth events remain unlinkable across days.

The change is probed on a throwaway project first — the deployed collector has
lagged its source before.

## 9. Test and documentation surface

- `analytics.test.ts` (batch carries `$user_id`, reset on logout, opt-out
  silences both sinks), `analyticsId` vs webcrypto, `analyticsProfile` counts,
  `metrics.test.ts`, a ProfilePage toggle test.
- Go: `update-analytics` handler and use-case tests, the middleware
  read-only-allowlist test, the migration command's idempotency test.
- `internal/test/apiparity` scenario for the new route plus golden regeneration;
  `mcpparity` `get_user` goldens (the options array grows by one, and every
  current-user payload gains `createdAt`); `make swagger`.
- i18n keys in all 11 catalogues.
- `docs/regression-test-plan.md` items for the toggle.
- `.env.example` and CLAUDE.md: remove `ECONUMO_ANALYTICS`, document the
  migration's one-time read, describe identified analytics and the per-user
  preference.
- A drafted privacy-policy revision for econumo.com (that site is not in this
  repo): account-linked usage events, the instance identifier and account
  profile, legitimate interest, the in-app opt-out, a new effective date.

## 10. Non-goals

No consent banner or consent gate. No `$user_name`. No
`host_kind` attribute (`deployment` carries the only distinction asked for; the
four-value variant that would have shown deployment shape was dropped). No
server-side pageviews. No changes to how cloud hostnames are reported.
