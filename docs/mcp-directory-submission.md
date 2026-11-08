# Submitting Econumo's MCP server to the Claude connectors directory

The connector published to Claude's directory is the **cloud instance only**:
`https://app.econumo.com/mcp`. A directory listing takes one fixed `https://`
URL; the "Multiple URLs" and "URL pattern" options a self-hosted fleet would
need can only be published once a listing is *Verified*, and Verified is not
something you apply for — Anthropic grants it during review. Self-hosters add
their own instance as a custom connector by URL, which already works and is
what `docs/mcp-setup.md` documents.

Portal: <https://claude.ai/directory/manage> > **Submit new** > **MCP
connector**. Escalations: `mcp-review@anthropic.com`. The submitter needs a
paid plan (Owner on Team, or the Directory permission on Enterprise).

## What already satisfies the review criteria

Verified against the code, not assumed:

| Requirement | Where it is met |
| --- | --- |
| OAuth 2.1 with DCR (RFC 7591) | `internal/authserver/register.go`; metadata advertises `registration_endpoint` |
| PKCE S256 mandatory | `internal/authserver/pkce.go`; `code_challenge_methods_supported: ["S256"]` |
| Protected-resource metadata (RFC 9728) | `resource` = `<issuer>/mcp`, matching the URL the user types |
| `authorization_servers` issuer first | Claude reads only the first entry and does not fall back |
| Public clients allowed | `"none"` in `token_endpoint_auth_methods_supported` |
| 401 + `WWW-Authenticate: Bearer resource_metadata=…` | Required: Claude ignores the header on a 200 |
| Loopback redirect, port ignored | Needed by Claude Code's ephemeral-port callback |
| Refresh-token rotation | Rotates every use; the replaced hash is retained to detect theft |
| Read/write split, no catch-all | Separate create/update/delete tools; no `api_request`-style tool with a `method` parameter |
| Actionable errors | `webmcp.MapErr` returns the REST envelope's coded, translated message plus per-field errors — never a bare "Internal Server Error" |
| Tool titles + hints on all 50 tools | `internal/web/mcp/annotations.go`, guarded by `TestEveryToolIsAnnotated` |
| `clone_budget` cannot overwrite an existing budget | `internal/budget/clone.go` rejects a `new_id` already in use |

Two notes on things that look like gaps and are not:

- `scopes_supported` lists only `mcp`, not `offline_access`. Harmless: the
  token endpoint issues a refresh token unconditionally
  (`internal/authserver/token.go`), so Claude gets one without asking.
- Anthropic's egress is `160.79.104.0/21`. The cloud instance must not have a
  WAF rule that blocks it from reaching discovery, `/oauth/register` or
  `/oauth/token`. Discovery, registration and token calls time out at 10s;
  refresh at 30s.

## Tool annotations

Anthropic requires a `title` plus the applicable hint on every tool, and the
hints drive auto-permissions: a read-only tool runs with no per-call
confirmation, a destructive one always prompts.

**The trap:** `destructiveHint` defaults to **true** whenever `readOnlyHint` is
false. Annotating only the reads would leave all 28 ordinary writes prompting
on every call. Register tools through `webmcp.ReadOnlyTool` /
`webmcp.WriteTool` / `webmcp.DestructiveTool` and this is right by
construction; `TestEveryToolIsAnnotated` fails the build otherwise.

The rule: **a tool is destructive if it deletes data, overwrites it in bulk, or
can silently destroy data the caller never mentioned.** That last clause is
what separates `update_transaction` from `update_category`.

| Hint | Count | Tools |
| --- | --- | --- |
| `readOnlyHint: true` | 13 | every `list_*` plus `get_budget`, `get_budget_plan`, `get_user` |
| `destructiveHint: true` | 11 | `delete_transaction`, `delete_budget_comment`, the four `merge_*`, `add_budget_account`, `remove_budget_account`, `bulk_update_transactions`, `update_transaction`, `update_envelope` |
| `destructiveHint: false` | 26 | creates, renames, `set_*_archived`, `set_limit`, `move_element`, `clone_budget`, `archive_budget`/`unarchive_budget`, `update_budget`, `update_budget_comment`, `update_folder`, `update_category`/`_label`/`_payee`/`_tag` |

The four `merge_*` tools delete the source after moving its references. Both
`*_budget_account` tools can destroy a savings row's plans and comments.
`bulk_update_transactions` overwrites up to 100 rows in one call.
`update_transaction` and `update_envelope` send full replacement state, so a
call that omits a field silently drops it — `update_transaction`'s own
`category_id` schema says omitting it "CLEARS any category the transaction
currently has", and `update_envelope` ungroups every category left out of
`category_ids`.

The 26 in the last row overwrite only the fields the caller names: the other
`update_*` tools require every field they touch, and `set_limit`'s clearing is
the documented purpose of omitting `amount`. That keeps `create_transaction` —
the daily "log 27.50 groceries" path — promptless.

A tidier fix for the two full-replace tools would be to make their replacement
fields pointers, so omitting one means "leave it alone" (the pattern
`update_transaction`'s own `label_ids` already uses) rather than "clear it".
That changes the tools' input contract, so it is left as a follow-up; the
destructive hint is the safe reading until then.

## The payments misread

Anthropic's unsupported-use-case list includes "Transfer money, cryptocurrency,
or other financial assets". Econumo records money that has **already** moved;
it has no payment rail. A reviewer skimming `create_transaction` could read
"transfer" the wrong way, so both the server `instructions` and that tool's
description now say up front that this is bookkeeping, that no real money
moves, and that a transfer is a ledger entry between two of the user's own
accounts. Keep that wording in the listing copy below.

## Open risk: unbounded `list_transactions`

The review criteria say to "keep responses reasonably sized".
`list_transactions` takes `account_id`, a period and the classification
filters, but **no `limit` and no pagination** — called bare on a long-lived
account it returns the entire history in one response. The SPA does not hit
this (it syncs per account through `get-account-transaction-list`), and the
tool description does list the filters, so a model normally narrows it.

Left as is deliberately: adding a `limit` changes the tool's contract and is a
product decision, not part of preparing the submission. Worth deciding before
submitting, because a reviewer escalated to Verified "runs a functional test of
each tool" against the populated test account. Two cheap options if it matters:
add an optional `limit` with a server-side default, or make the description
tell the model to pass a period for anything beyond a recent window.

## Draft listing copy

Review and edit — these are drafts, not decisions.

- **Name** (≤100): `Econumo`
- **Slug** (permanent once published): `econumo`
- **One-liner** (≤200): Track spending and run your budgets by asking Claude —
  log an expense, review where a month stands, or plan the one ahead.
- **Categories** (1–5): Finance first; Productivity as the likely second. Pick
  from the portal's own list.
- **Read/write**: both.
- **Description** (≤2000):

  > Econumo is a personal finance and budgeting app. Connect it and Claude can
  > keep your books with you: record what you spent or earned, see where a
  > month's budget actually stands, and plan the months ahead.
  >
  > Ask it to log an expense in plain language and it resolves the account,
  > category and payee for you. Ask how a month went and it reads your real
  > envelopes, limits and balances — across every currency you hold, converted
  > at the rates that applied in that month. Ask it to plan next month and it
  > can set limits envelope by envelope, group categories, and reorganise the
  > budget's structure. Shared budgets work too: comments on a budget cell are
  > readable and writable, so a household can keep one plan between them.
  >
  > Econumo is bookkeeping, not banking. It records money that has already
  > moved and never moves any itself — there is no payment or transfer-out
  > capability, and a "transfer" is an entry between two of your own accounts.
  >
  > Read tools run without interrupting you. Anything that deletes or rewrites
  > data in bulk — deleting a transaction, merging two categories, reclassifying
  > a batch — asks you first.
  >
  > Econumo is open source and self-hostable. This connector is the hosted
  > instance at app.econumo.com; if you run your own, add it to Claude as a
  > custom connector with your own URL.

## What only you can supply

- [ ] **Privacy policy URL** — confirm `app.econumo.com`/`econumo.com` has a
      published policy covering collection, use, storage, third-party sharing,
      retention and contact.
- [ ] **Documentation URL** — required by the publish date; a help-centre page
      or blog post is enough. Publishing `docs/mcp-setup.md` under
      `econumo.com` is the cheapest route.
- [ ] **Support contact**.
- [ ] **Icon** — the portal states no dimensions; have a square PNG ready.
- [ ] **Company name, website, primary contact**.
- [ ] **Test account on app.econumo.com, fully populated** — "a fully populated
      account rather than an empty shell". It needs several accounts in more
      than one currency, a budget with envelopes, folders, limits and savings
      rows, a few months of transactions, categories/payees/tags/labels, and a
      second connected user so `list_connections` and the budget-comment tools
      return something. Write the sign-in steps for someone who has never seen
      Econumo.
- [ ] **Run every tool yourself** — the Test & launch step asks you to confirm
      it. `npx @modelcontextprotocol/inspector --cli
      https://app.econumo.com/mcp --transport http --method tools/list`, then
      add the server to your own Claude account and check it reads **Connected**
      with the tools listed under Tool permissions.
- [ ] **Seven compliance acknowledgments** — directory guidelines, first-party
      API use, financial transactions, AI media generation, prompt injection,
      conversation-data collection, and public documentation. Econumo answers
      cleanly on all seven: it calls its own API on its own domain, moves no
      money, generates no media, carries no injected instructions in tool
      descriptions, and collects no conversation data.

## After submitting

Submissions are scanned automatically and listed as **Community** by default.
Statuses are Draft, In review, Changes requested, Not approved, Approved,
Published — and **Approved does not publish**; someone with edit rights has to
press publish. Editing a published listing sends it back to draft for another
review. There is no stated SLA: "review time isn't fixed."
