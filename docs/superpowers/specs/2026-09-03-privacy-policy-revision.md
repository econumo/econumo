# Privacy policy revision — product analytics

This is a draft of the section(s) of econumo.com's privacy policy that need to
change for identified product analytics. **econumo.com is not in this
repository** — this file is the proposed replacement wording, to be reviewed
and published on the site separately. It supersedes any earlier language that
described Econumo's product analytics as anonymous.

Where the existing policy has its own structure (definitions, a table of
processing activities, a "your rights" section, etc.), the wording below
should be adapted into that structure rather than pasted in verbatim. What
must not change in the adaptation is the substance of each numbered point.

---

## Proposed section: "Product analytics"

**Effective date: [DATE — to be filled in on publication]**

### What we collect and why

Econumo collects product usage analytics to understand how the app is used —
which features are useful, where people get stuck, and how usage changes over
time. As of the effective date above, this analytics is **linked to your
account** through a pseudonymous identifier: each event we record is tagged
with a value derived from your account by a one-way cryptographic hash, so
that events from the same account can be connected to each other over time.
We can recognize repeat activity from your account; we do not store your
account identifier, your name, or your email address alongside this data.

Specifically, we collect:

- **Feature usage** — which pages and features you use, and the actions you
  take in them (for example, creating a budget or connecting an account),
  recorded as event names without their financial content.
- **Account shape** — counts only: how many accounts, categories, payees,
  tags, and shared connections exist on your account (including archived or
  hidden ones), and how many are archived or hidden. We collect counts, not
  the underlying items.
- **When you signed up** — the month and year of your registration, not the
  exact date.
- **App version and platform** — the version of Econumo you are running and
  whether you are using it on the web, iOS, or Android.
- **Deployment type and instance** — whether you are using Econumo Cloud or a
  self-hosted installation, and, for a self-hosted installation, a technical
  identifier for that specific installation (see below). For Econumo Cloud,
  we also record which of our own subdomains you are using.
- **Interface details** — your selected display language, your subscription
  or access status (e.g., active vs. a lapsed trial), and coarse
  interface-mode information (e.g., desktop vs. mobile layout).

### What we do not collect

We do not send any of the following to our analytics system:

- Your financial data — no transaction amounts, descriptions, dates, or
  balances.
- The names of your accounts, categories, payees, or tags.
- The contents of any transaction.
- Your email address or your name.
- For self-hosted installations, the installation's hostname or network
  address. The "instance identifier" mentioned above is a one-way derived
  value with no relationship to your server's address; it exists only so we
  can tell installations apart from one another, not to locate or identify
  them.

### The pseudonymous identifier, plainly stated

We want to be direct about what "pseudonymous" means here: this is not
anonymous tracking. Because the identifier is derived from your account in a
way we can reproduce, we are able to connect the analytics we hold back to
your account if we need to — for example, to answer a support question about
what you were seeing when a problem occurred. Someone without access to our
systems could not do this from the analytics data alone, but we can. If you
would prefer we not collect this data about your account at all, see "Your
choice" below.

For a self-hosted installation used by only one person, this analytics data
about that installation is, in practice, data about that one person, and we
treat it accordingly.

### Legal basis

We process this data on the basis of our **legitimate interest** in
understanding and improving the product, balanced against your right to
object. Because that balance depends on you having a real, easy way to
object, we provide one directly in the product — see below — rather than
relying on a cookie-consent banner or similar mechanism.

### Your choice

You can turn this analytics off at any time from **Settings → Profile**, in
the "Privacy" section of that page. Turning it off takes effect immediately:
no further analytics events are sent from your account from that point
forward. This setting applies **per account** — if you have access to more
than one Econumo account (for example, a shared household), each account's
setting is independent, and turning it off for one account does not affect
the others.

This is the mechanism by which you exercise your right to object to this
processing under applicable law. If you turn analytics off and later change
your mind, you can turn it back on the same way.

### Retention and third parties

Analytics data is retained by our analytics provider, Twillingate, under
their own data-processing terms with us, and is not sold or shared with
advertisers. [Retention period and any additional Twillingate-specific
disclosures to be confirmed against our data-processing agreement before
publication.]

---

## Notes for whoever publishes this (not policy text)

- The "[DATE — to be filled in on publication]" placeholder should be set to
  the day the new analytics behavior actually starts sending identified
  events in production, not the day the policy is edited.
- The retention/third-party paragraph has an open bracket that needs the
  Twillingate DPA retention terms filled in before this goes live — do not
  publish with the placeholder still in place.
- This wording was drafted against the implementation in
  `docs/superpowers/specs/2026-09-03-identified-analytics-design.md`; if that
  design changes before shipping (e.g. the attribute list, what counts are
  sent, or the opt-out's scope), re-check this draft against the new
  behavior before publishing rather than assuming it still matches.
