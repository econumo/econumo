# Apple Wallet shortcuts — build guide

Two signed shortcuts ship as static assets in `web/public/shortcuts/`:

| Shortcut name (exact) | Served as | Job |
|---|---|---|
| `Econumo Setup` | `econumo-setup-v1.shortcut` | Receives `{url, token}` from a `shortcuts://run-shortcut` deep link and writes it to `Shortcuts/econumo-wallet.json` in iCloud Drive. |
| `Econumo Wallet` | `econumo-wallet-v1.shortcut` | Run by the user's Transaction automation: reads that file and POSTs the tap to `ingest-apple-wallet-event`. |

The names are part of the contract: the Settings → Import & export page opens
`shortcuts://run-shortcut?name=Econumo%20Setup&input=text&text=…`, and the
setup instructions tell the user to point their automation at
`Econumo Wallet`. Renaming either means changing the SPA.

`.shortcut` files must be signed, and signing only works on a Mac, so the
files are built by hand in Shortcuts.app and committed. The unsigned plist
source is committed next to this README so the recipe is reviewable in
diffs; regenerate both whenever the recipe changes and bump the `-vN`
suffix. The same recipe, reduced to the five actions a user can type in, is
shown in the SPA under *Configure manually*
(`web/src/features/imports/AppleWalletSetup.tsx`); keep the URL, headers and
field names in the two in sync.

## Contract with the server

Configuration file `Shortcuts/econumo-wallet.json` (written by Setup, read
by Wallet):

```json
{ "url": "https://econumo.example.com", "token": "eco_pat_…" }
```

`url` has no trailing slash. `token` is an `'ingest'`-scoped personal
access token; the SPA mints it at the moment the user taps Configure.

Request made by Wallet on every tap:

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
  "type":       "expense",
  "eventId":    "<Transaction Identifier, optional>"
}
```

`eventId` is optional: when present it is the dedupe key (a re-run of the
automation for the same tap is `duplicate`); when absent the server derives
`sha256(lower(account)|occurredAt UTC|amount|payee)`. `type` is `expense` or
`income` (refunds); anything else is a parse error. `payee` is stored up to
255 characters.

The server answers 200 `{"status": "created"|"queued"|"skipped"|"duplicate"|"failed", "eventId": "…"}`
whenever the event was stored, even if it could not be imported (`queued` /
`failed` rows are handled on the web queue page), so the shortcut has nothing
to do with the response. Non-200s: 401 (token revoked — run Setup again),
402 (read-only access), 429 (the per-user ingest rate limit,
`ECONUMO_RATE_LIMIT_INGEST`).

## Build: `Econumo Setup`

Shortcuts.app → File → New Shortcut. Name it exactly `Econumo Setup`.

Shortcut details (the ⓘ panel): leave "Show in Share Sheet" off. The input
arrives as **Shortcut Input** from the `run-shortcut` URL, no share-sheet
setup needed.

Actions, in order:

1. **Get Dictionary from Input** — input: `Shortcut Input`.
   (Parses the JSON text the deep link carries.)
2. **Get Dictionary Value** — Get `Value` for `url` in `Dictionary`.
3. **If** — `Dictionary Value` `does not have any value`
   1. **Show Alert** — title `Econumo`, message
      `Invalid configuration. Open Settings → Import & export in Econumo and tap Configure again.`,
      "Show Cancel Button" off.
   2. **Stop This Shortcut**
   
   **End If**
4. **Set Name** — input: `Shortcut Input`, name `econumo-wallet.json`,
   "Don't Include File Extension" off.
5. **Save File** — input: `Renamed Item`; Service: `iCloud Drive`;
   "Ask Where to Save": **off**; Destination Path: `Shortcuts/econumo-wallet.json`;
   "Overwrite If File Exists": **on**.
6. **Show Notification** — `Econumo configured for` + `Dictionary Value`
   (the `url` value from step 2).

## Build: `Econumo Wallet`

File → New Shortcut. Name it exactly `Econumo Wallet`. "Show in Share
Sheet" off.

Actions, in order:

1. **Get File** — Service: `iCloud Drive`; File Path:
   `Shortcuts/econumo-wallet.json`; "Error If Not Found": **off**.
2. **If** — `File` `does not have any value`
   1. **Show Notification** — `Econumo is not configured. Open Settings → Import & export in Econumo and tap Configure.`
   2. **Stop This Shortcut**
   
   **End If**
3. **Get Dictionary from Input** — input: `File`.
4. **Get Dictionary Value** — `url` in `Dictionary` → rename the output
   variable to `ServerURL` (tap the variable → Rename).
5. **Get Dictionary Value** — `token` in `Dictionary` → rename to `Token`.
6. **Format Date** — Date: `Shortcut Input` › **Date** property; Date
   Format: `ISO 8601`; "Include ISO 8601 Time": **on**.
   (Gives `occurredAt` with the device's offset, which is what the server
   uses for the wall-clock date.)
7. **Get Contents of URL**
   - URL: `ServerURL` followed by the literal text
     `/api/v1/import/ingest-apple-wallet-event`
     (type the variable, then the path, in the same field).
   - Method: `POST`.
   - Headers: `Authorization` = the text `Bearer ` (with the trailing space)
     followed by the `Token` variable.
   - Request Body: `JSON`, fields:

     | Key | Type | Value |
     |---|---|---|
     | `account` | Text | `Shortcut Input` › **Card or Pass** |
     | `payee` | Text | `Shortcut Input` › **Merchant** |
     | `amount` | Text | `Shortcut Input` › **Amount** |
     | `currency` | Text | `Shortcut Input` › **Amount** › **Currency Code** (see note) |
     | `occurredAt` | Text | `Formatted Date` (step 6) |
     | `type` | Text | `expense` |
     | `eventId` | Text | `Shortcut Input` › **Transaction Identifier** (omit the row if the picker has no such property) |

   Use the Text type for every field, including `amount`: the server parses
   the decimal itself and tolerates locale formatting; a Number field would
   let Shortcuts round or localize it.

Nothing after the request. The automation runs unattended; there is no one
to show a result to.

**Property names note.** The Transaction object's properties are chosen
from the variable picker (tap `Shortcut Input` → pick the property). The
names above are the ones the picker shows on iOS 17/18; if the currency
code is not offered as a sub-property of Amount, the Transaction object
itself exposes it (look for "Currency" in the picker). Whatever the picker
calls it, the field must end up as a 3-letter ISO code, never a symbol —
the server rejects `$` as ambiguous.

**Refunds.** Wallet reports a refund as a transaction with type Refund. v1
sends `expense` for everything; a refund therefore imports as an expense
with a positive amount and needs a hand fix. Mapping Refund → `income` is
an `If` on `Shortcut Input` › **Type** and is a follow-up once the property
is confirmed on a device.

## Same-named cards (per-card variant)

Two cards with the same display name are indistinguishable to the server,
because `account` is the card name. The user-side fix: duplicate
`Econumo Wallet` (right-click → Duplicate), rename it (`Econumo Wallet —
Visa joint`), replace the `account` value with a typed literal, and point a
Transaction automation filtered to that one card at the copy. Nothing
changes server-side; the literal is just another external account name.

## Test on a device before signing

1. AirDrop or iCloud-sync both shortcuts to an iPhone.
2. In Safari on the phone, open Econumo → Settings → Import & export → Configure.
   Shortcuts should open, run `Econumo Setup`, and show the notification.
   Check `Files` → iCloud Drive → Shortcuts → `econumo-wallet.json`.
3. Create the automation: Shortcuts → Automation → `+` → **Transaction** →
   any card, any type → **Run Immediately** → `Econumo Wallet`.
4. Make an Apple Pay purchase (or a refund) and watch the event appear on
   the Econumo queue page or as an imported transaction.
5. Delete the JSON file and tap a card again: the "not configured"
   notification must appear and nothing must be posted.

Known things to confirm during this test (the spec lists them as device
tests): `Get File` works inside a background Transaction run, and
`Save File` on a phone with iCloud Drive turned off (fallback: the
on-device Shortcuts folder).

## Sign and export

Each shortcut, in Shortcuts.app on the Mac:

1. Select it → **File → Export…** (or the share button → *Export File…*).
2. When asked who can import, choose **Anyone**. This notarizes the file
   through iCloud so any device accepts it. If the dialog has no such
   choice, export the file and run:

   ```
   shortcuts sign --mode anyone --input "Econumo Wallet.shortcut" --output econumo-wallet-v1.shortcut
   ```

3. Save as `econumo-setup-v1.shortcut` / `econumo-wallet-v1.shortcut` into
   `web/public/shortcuts/`.
4. Extract the unsigned source for review, into this directory:

   ```
   shortcut-sign extract -i web/public/shortcuts/econumo-wallet-v1.shortcut -o web/shortcuts/econumo-wallet.plist
   plutil -convert xml1 web/shortcuts/econumo-wallet.plist
   ```

   (`shortcut-sign`: https://github.com/0xilis/shortcut-sign — the `extract`
   subcommand only reads the archive, it needs no key material.)

5. Verify by opening the served file on an iPhone in Safari: the "Add
   Shortcut" sheet must appear without a "cannot be opened because it is
   not signed" error.

Bump `-v1` to `-v2` on the next change and update the links in the SPA;
the old file may stay so an already-installed shortcut keeps its download
link working.
