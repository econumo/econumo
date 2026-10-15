# Apple Wallet shortcut — build guide

One signed shortcut ships as a static asset in `web/public/shortcuts/`:

| Shortcut name = file name | Served as | Job |
|---|---|---|
| `econumo-apple-wallet-v2` | `econumo-apple-wallet-v2.shortcut` | Run with `{url, token}` text from the SPA's `shortcuts://run-shortcut` deep link: writes it to `econumo-wallet.json` in the iCloud Drive › Shortcuts folder and checks in with the server once. Run by the user's Transaction automation: reads that file and POSTs the tap to `ingest-apple-wallet-event`. |

v1 shipped the two jobs as two shortcuts, `econumo-setup-v1` and
`econumo-wallet-v1`. Their signed files stay in `web/public/shortcuts/` so
existing setups keep a working download link, but nothing builds or links
them any more; the server contract below is unchanged, so a v1 automation
keeps working.

iOS names an imported shortcut after the file it came from and ignores the
embedded `WFWorkflowName`, so the shortcut name IS the served basename
(`build.py` derives it from the slug and `VERSION`). The name is part of the
contract: the Settings → Apple Wallet page opens
`shortcuts://run-shortcut?name=econumo-apple-wallet-v2&input=text&text=…`, and the
setup instructions tell the user to point their automation at
`econumo-apple-wallet-v2` (`WALLET_SHORTCUT_NAME` in
`web/src/features/imports/AppleWalletSetup.tsx`, plus the
`imports.apple_wallet.steps.*` catalogue strings). A `VERSION` bump therefore
renames the shortcut too: update the SPA constant and the catalogues in the
same commit, and expect users to reinstall and repoint their automation.

## How the files are made

Nothing is built by hand. `build.py` (stdlib Python, no dependencies) holds
the recipe as code, writes the unsigned XML plist source next to it
(`econumo-apple-wallet.plist` — the reviewable artifact in diffs) and signs it into
`web/public/shortcuts/`:

```
python3 apple/shortcut/build.py            # plists + signed files
python3 apple/shortcut/build.py --no-sign  # plists only (any OS)
```

Signing is `shortcuts sign --mode anyone` — the Apple CLI that ships with
macOS, run on a Mac whose Shortcuts.app is signed into an Apple ID. "Anyone"
notarizes the file through iCloud so any device accepts it. The signed
container is an `AEA1` archive wrapping the same plist; there is no need to
extract anything from it because the plist source is what is committed.

Action UUIDs are deterministic (uuid5 of a per-shortcut counter), so
rebuilding without a recipe change produces a byte-identical plist and only
the signature bytes differ. When the recipe changes, bump `VERSION` in
`build.py` (`-v2` → `-v3`), rebuild, and update the names and links in the SPA and
catalogues; the old file may stay so an already-installed shortcut keeps
its download link working.

The same recipe, reduced to the actions a user can type in, is the
illustrated guide that the SPA's *Configure manually* link opens
(`MANUAL_GUIDE_URL` in `web/src/features/imports/AppleWalletSetup.tsx`,
`https://econumo.com/docs/user-guide/apple-wallet`); keep the URL, headers
and field names in the two in sync.

Keep `build.py`, the plist and the signed file in the same commit.

## Contract with the server

Configuration file `econumo-wallet.json` in the Shortcuts folder of iCloud
Drive (written by the setup run, read by every run — both `Save File` and `Get File`
resolve paths relative to that folder, so the path in the recipe is the bare
file name):

```json
{ "url": "https://econumo.example.com", "token": "eco_pat_…" }
```

`url` has no trailing slash. `token` is an `'ingest'`-scoped personal
access token; the SPA mints it at the moment the user taps "Create access
token".

Request made on every tap:

```
POST {url}/api/v1/import/ingest-apple-wallet-event
Authorization: Bearer {token}
Content-Type: application/json

{
  "account":    "<Card or Pass name>",
  "payee":      "<Merchant>",
  "amount":     "<Amount, number>",
  "currency":   "<ISO 4217 code>",
  "occurredAt": "<ISO 8601 with offset>",
  "type":       "expense"
}
```

The server answers 200 whenever the event was stored, even if it could not
be imported (`queued` / `skipped` / `failed` are visible in the web UI), so
the shortcut has nothing to do with the response.

## Recipe: `econumo-apple-wallet-v2`

"Show in Share Sheet" off. The shortcut branches on what it was given:
the setup deep link passes **text**, the Transaction automation passes a
**wallet transaction**, and a run by hand passes **nothing**.

1. **Get Type** — input: `Shortcut Input`.
2. **If** — `Type` `is` `Text` (no Otherwise) — the setup run:
   1. **Get Dictionary from Input** — input: `Shortcut Input`.
      (Parses the JSON text the deep link carries.)
   2. **Get Dictionary Value** — Get `Value` for `url` in `Dictionary`.
   3. **If** — `Dictionary Value` `does not have any value` (no Otherwise)
      1. **Show Alert** — title `Econumo`, message
         `Invalid configuration. Open Settings → Apple Wallet in Econumo and tap Configure again.`,
         "Show Cancel Button" off.
      2. **Stop This Shortcut**

      **End If**
   4. **Set Name** — input: `Shortcut Input`, name `econumo-wallet.json`,
      "Don't Include File Extension" off.
   5. **Save File** — input: `Renamed Item`; "Ask Where to Save": **off**;
      Destination Path: `econumo-wallet.json` (inside iCloud Drive › Shortcuts);
      "Overwrite If File Exists": **on**.
   6. **Read the configuration** — steps 3–7 below, on the file just saved.
      Reading it back rather than reusing the input matters: iOS tracks the
      privacy grants by data provenance, and the automation's request carries
      data from that file (see *Privacy grants*).
   7. **Get Contents of URL** — the request below with the JSON body
      `{"check": "setup"}` (Text). It has no `account`, so the server stores
      a failed event with the error `account is required` — the SPA's proof
      that the shortcut reached it.
   8. **Show Notification** — `Econumo configured for` + the `url` value.
   9. **Stop This Shortcut**

   **End If**
3. **Get File** — File Path: `econumo-wallet.json` (iCloud Drive ›
   Shortcuts); "Show Document Picker" off; "Error If Not Found": **off**.
4. **If** — `File` `does not have any value` (no Otherwise)
   1. **Show Notification** — `Econumo is not configured. Open Settings → Apple Wallet in Econumo and tap Configure.`
   2. **Stop This Shortcut**

   **End If**
5. **Get Dictionary from Input** — input: `File`.
6. **Get Dictionary Value** — `url` in `Dictionary`.
7. **Get Dictionary Value** — `token` in `Dictionary`.
8. **Format Date** — Date: `Current Date`; Date Format: `ISO 8601`;
   "Include ISO 8601 Time": **on**.
   The Transaction object exposes no date property (see below), and the
   automation runs within seconds of the tap, so the device's "now" — with
   its UTC offset, which is what the server uses for the wall-clock date —
   is the event time.
9. **Get Contents of URL**
   - URL: the `url` value (step 6) followed by the literal text
     `/api/v1/import/ingest-apple-wallet-event`.
   - Method: `POST`.
   - Headers: `Authorization` = the text `Bearer ` (trailing space)
     followed by the `token` value (step 7).
   - Request Body: `JSON`, fields:

     | Key | Type | Value |
     |---|---|---|
     | `account` | Text | `Shortcut Input` › **Card or Pass** |
     | `payee` | Text | `Shortcut Input` › **Merchant** |
     | `amount` | Text | `Shortcut Input` as **Currency Amount** › **Currency Amount** |
     | `currency` | Text | `Shortcut Input` as **Currency Amount** › **Currency Code** |
     | `occurredAt` | Text | `Formatted Date` (step 8) |
     | `type` | Text | `expense` |

   Every field is Text, including `amount`: the server parses the decimal
   itself and tolerates locale formatting; a Number field would let
   Shortcuts round or localize it.

Nothing after the request. The automation runs unattended; there is no one
to show a result to. A run by hand reaches step 9 with no input, so it posts
no `account` and lands as the same `account is required` proof.

**Where the property names come from.** The Transaction trigger hands the
shortcut a `WFWalletTransactionContentItem` (ContentKit, iOS 17+). That
class exposes exactly three properties — `Card or Pass`, `Merchant`,
`Amount` — and coerces to a `WFCurrencyAmountContentItem`, whose properties
are `Currency Code` and `Currency Amount`. The recipe therefore reads the
card and merchant as properties of Shortcut Input, and the amount and
currency by coercing Shortcut Input to a Currency Amount first (in the app:
tap the variable, change "Get" from Transaction to Currency Amount, then
pick the property). That is what yields a bare number and a 3-letter ISO
code rather than a formatted `$4.50`; the server rejects a symbol as
ambiguous. There is no date and no type/refund property on the object.

**Refunds.** Wallet reports a refund as a transaction with type Refund, but
the object exposes no type property, so the shortcut sends `expense` for everything; a
refund imports as an expense with a positive amount and needs a hand fix.

## Same-named cards (per-card variant)

Two cards with the same display name are indistinguishable to the server,
because `account` is the card name. The user-side fix: duplicate
`econumo-apple-wallet-v2` (right-click → Duplicate), rename it (`econumo-apple-wallet-v2
Visa joint`), replace the `account` value with a typed literal, and point a
Transaction automation filtered to that one card at the copy. Nothing
changes server-side; the literal is just another external account name.

## Privacy grants — why the setup run checks in

iOS grants a shortcut's privacy permissions **per shortcut** and tracks them
by data provenance, and an unattended run cannot collect one. An automation
set to Run Immediately that still lacks a grant fails on the lock screen with
*"This shortcut requires privacy permissions that cannot be granted while
your device is locked"* — nothing is posted and nothing is logged
server-side. Run After Confirmation is what makes the first payment work: the
payment raises a prompt to run the shortcut, and answering it lets iOS ask
for the missing grants, even if the phone was locked when the user paid.
`econumo-apple-wallet-v2` needs three grants:

1. **Access to the server host** — asked the first time `Get Contents of
   URL` contacts `{url}`.
2. **iCloud Drive** — the request body is derived from the config file, so
   *Get Contents of URL* asks to use iCloud Drive data too.
3. **Wallet** — the body also carries transaction-derived data (Card or
   Pass, Merchant, Amount); this one only appears during a real transaction,
   because a manual run has no Transaction input.

The setup run collects 1 and 2: it is started by hand from the SPA (the
phone is unlocked), and its check-in request uses `url` and `token` read
back from the saved file — the same provenance as the automation's request —
so "Always Allow" there covers the automation too. Grant 3 waits for the
first Apple Pay payment: the automation is created with **Run After
Confirmation**, so the user taps Run on the payment's prompt and answers
"Always Allow" (on 3) — the phone may stay locked until then. If iOS still
asks for 1 or 2 at that payment, the provenance assumption did not hold on
that iOS version; the same prompt collects them, so setup still completes.
Only after that does the guide switch the automation to Run Immediately.

The check-in has no `account` and is stored as a failed event with the
literal error `account is required` (`internal/imports/applewallet/parser.go`);
while the guide's connect step is open the SPA polls the queue, treats that
event as proof of contact and discards it. Keep that error text stable — the
SPA matches on it. A run by hand produces the same event.

## Test on a device before shipping

The plist is generated from documented action formats; the type branch,
the check-in, the Transaction property names, the coercion, and the ISO
date output are only proven by this pass.

1. Open the served `econumo-apple-wallet-v2.shortcut` on an iPhone in Safari (or
   AirDrop it). The "Add Shortcut" sheet must appear without a "cannot be
   opened because it is not signed" error. After adding, the name in the
   library is `econumo-apple-wallet-v2`.
2. In Safari on the phone, open Econumo → Settings → Apple Wallet → step 2,
   tap "Create access token", then "Open econumo-apple-wallet-v2". Shortcuts
   should run it, ask for the server host and iCloud Drive (answer **Always
   Allow**), and show "Econumo configured for …". Check `Files` → iCloud
   Drive → Shortcuts → `econumo-wallet.json`; the server log shows
   `ingest-apple-wallet-event … ingest_status=failed`, and the step in the
   SPA ticks itself within a few seconds of switching back.
3. Create the automation: Shortcuts → Automation → `+` → **Transaction** →
   any card, any type → **Run After Confirmation** → `econumo-apple-wallet-v2`.
4. Make an Apple Pay purchase (the phone may be locked), tap Run on the
   automation's prompt, answer
   Always Allow to the Wallet prompt (note whether host / iCloud Drive are
   asked again — they should not be), and watch the event appear on the
   Econumo queue page or as an imported transaction. Check the stored
   event: card name and merchant filled, `amount` a bare number, `currency`
   a 3-letter code, `occurredAt` carrying the device's offset (e.g.
   `+02:00`). Switch the automation to **Run Immediately** and pay once
   more from the lock screen: it must arrive without a prompt.
5. Delete the JSON file and tap a card again: the "not configured"
   notification must appear and nothing must be posted.

Confirmed on device for v1: `Get File` works inside a background Transaction
run once the grants above are in place. Still open: `Save File` on a phone
with iCloud Drive turned off (fallback: the on-device Shortcuts folder).
