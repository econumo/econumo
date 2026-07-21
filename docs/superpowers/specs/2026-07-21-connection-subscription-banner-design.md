# Connection subscription notifications in the SubscriptionBanner

**Date:** 2026-07-21
**Status:** Approved (Approach A)

## Problem

A connection's (partner's) subscription running out is invisible outside the
Shared access settings page. When the partner slides into read-only, their
edits to shared accounts silently stop — the family finds out by breakage.

## Decision

Extend the existing `SubscriptionBanner` (already mounted once in
`ApplicationLayout`) to cover connection states. Frontend-only: the data is
already served by `get-connection-list`; no backend changes.

## Behavior

### Variants and priority

One banner at a time. Priority (highest wins):

1. `readonly` — own account read-only (existing; permanent, non-dismissible)
2. `trial` — own subscription ends in ≤3 days (existing; dismissible per day)
3. `partner_readonly` — a connection is read-only (new; dismissible per day)
4. `partner_trial` — a connection's subscription ends in ≤3 days (new; dismissible per day)

Within partners: read-only beats ending-soon; among several ending trials the
soonest expiry wins. The banner names exactly one partner.

### Gating

- Own variants: unchanged (trial requires `billingEnabled`; readonly always shows).
- Partner variants require `billingEnabled` — the CTA is the point, and the
  billing portal can pay for every family member. Self-hosted instances
  (no `BILLING_URL`) fetch nothing and show nothing new; partner status
  remains visible on the Shared access page as today.

### Data

`SubscriptionBanner` calls `useConnections({ enabled: billingEnabled })` —
the existing query (staleTime 10 min, no polling). A failed/empty connections
query simply yields no partner variant; no error UI.

Derivation reuses `deriveAccessState` / `accessDaysLeft` from `lib/access`
exactly as `ConnectionsPage` does.

### Dismissal

The existing one-local-day mechanism (`subscriptionBannerDismissedDay` in
localStorage) applies to all dismissible variants with the single shared key:
priority means only one banner shows at a time, so per-variant keys would
resurface a second nag immediately after dismissing the first. Own readonly
stays permanent and non-dismissible.

### CTA

Same "Manage subscription" button on every variant, opening the caller's own
billing portal via `useOpenBillingPortal().open()` (no `forUserId`): the
portal handles paying for everyone. The per-partner `pay_for` CTA on the
Shared access page is unchanged.

### Copy (en; ru mirrors with 3-form plurals)

- `subscription.banner.connection_trial`:
  `"{name}'s subscription ends in {count} day | {name}'s subscription ends in {count} days"`
- `subscription.banner.connection_readonly`:
  `"{name}'s subscription has ended — they can view shared accounts but not edit"`

### Analytics

Reuse `SUBSCRIPTION_BANNER_SHOW` with new `variant` property values
`partner_trial` / `partner_readonly`. No new metric key.

## Testing

Extend `SubscriptionBanner.test.tsx`:

- partner trial ≤3 days → banner with the partner's name + CTA, metric fires
  with `variant: 'partner_trial'`
- partner trial 30 days out → nothing
- partner readonly → banner; own trial ≤3 days simultaneously → own trial wins
- dismissal hides partner variants for the day (shared key)
- billing disabled → no partner variants (and no connections fetch)
- i18n guards (`internal/test/i18ntest`) pick up the new keys automatically

## Out of scope (future)

- Server-side email notifications ("Megan's access ends in 3 days") — needs
  scheduled sends, per-user tracking, unsubscribe; separate project.
- Backend-computed attention flags in `get-user-data` (rejected: frozen-contract
  churn across goldens/parity for one cached GET).
