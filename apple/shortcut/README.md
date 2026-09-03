# Apple Wallet shortcuts — build guide

Two signed shortcuts ship as static assets in `web/public/shortcuts/`:

| Shortcut name = file name | Served as | Job |
|---|---|---|
| `econumo-setup-v1` | `econumo-setup-v1.shortcut` | Receives `{url, token}` from a `shortcuts://run-shortcut` deep link and writes it to `econumo-wallet.json` in the iCloud Drive › Shortcuts folder. |
| `econumo-wallet-v1` | `econumo-wallet-v1.shortcut` | Run by the user's Transaction automation: reads that file and POSTs the tap to `ingest-apple-wallet-event`. |

iOS names an imported shortcut after the file it came from and ignores the
embedded `WFWorkflowName`, so the shortcut names ARE the served basenames
(`build.py` derives both from the slug and `VERSION`). The names are part of
the contract: the Settings → Import & export page opens
`shortcuts://run-shortcut?name=econumo-setup-v1&input=text&text=…`, and the
setup instructions tell the user to point their automation at
`econumo-wallet-v1` (`WALLET_SHORTCUT_NAME` / `SETUP_SHORTCUT_NAME` in
`web/src/features/imports/AppleWalletSetup.tsx`, plus the
`imports.apple_wallet.steps.*` catalogue strings). A `VERSION` bump therefore
renames the shortcuts too: update the SPA constants and the catalogues in
the same commit, and expect users to reinstall and repoint their automation.

## How the files are made

Nothing is built by hand. `build.py` (stdlib Python, no dependencies) holds
the recipe for both shortcuts as code, writes the unsigned XML plist source
next to it (`econumo-setup.plist`, `econumo-wallet.plist` — the reviewable
artifact in diffs) and signs them into `web/public/shortcuts/`:

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
`build.py` (`-v1` → `-v2`), rebuild, and update the names and links in the SPA and
catalogues; the old file may stay so an already-installed shortcut keeps
its download link working.

The same recipe, reduced to the five actions a user can type in, is shown in
the SPA under *Configure manually*
(`web/src/features/imports/AppleWalletSetup.tsx`); keep the URL, headers and
field names in the two in sync.

Keep `build.py`, the two plists and the two signed files in the same commit.

## Contract with the server

Configuration file `econumo-wallet.json` in the Shortcuts folder of iCloud
Drive (written by Setup, read by Wallet — both `Save File` and `Get File`
resolve paths relative to that folder, so the path in the recipe is the bare
file name):

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
  "type":       "expense"
}
```

The server answers 200 whenever the event was stored, even if it could not
be imported (`queued` / `skipped` / `failed` are visible in the web UI), so
the shortcut has nothing to do with the response.

## Recipe: `econumo-setup-v1`

What `build.py` emits, in Shortcuts.app terms. Input arrives as
**Shortcut Input** from the `run-shortcut` URL; "Show in Share Sheet" is off.

1. **Get Dictionary from Input** — input: `Shortcut Input`.
   (Parses the JSON text the deep link carries.)
2. **Get Dictionary Value** — Get `Value` for `url` in `Dictionary`.
3. **If** — `Dictionary Value` `does not have any value` (no Otherwise)
   1. **Show Alert** — title `Econumo`, message
      `Invalid configuration. Open Settings → Import & export in Econumo and tap Configure again.`,
      "Show Cancel Button" off.
   2. **Stop This Shortcut**

   **End If**
4. **Set Name** — input: `Shortcut Input`, name `econumo-wallet.json`,
   "Don't Include File Extension" off.
5. **Save File** — input: `Renamed Item`; "Ask Where to Save": **off**;
   Destination Path: `econumo-wallet.json` (inside iCloud Drive › Shortcuts);
   "Overwrite If File Exists": **on**.
6. **Show Notification** — `Econumo configured for` + `Dictionary Value`
   (the `url` value from step 2).

## Recipe: `econumo-wallet-v1`

"Show in Share Sheet" off.

1. **Get File** — File Path: `econumo-wallet.json` (iCloud Drive ›
   Shortcuts); "Show Document Picker" off; "Error If Not Found": **off**.
2. **If** — `File` `does not have any value` (no Otherwise)
   1. **Show Notification** — `Econumo is not configured. Open Settings → Import & export in Econumo and tap Configure.`
   2. **Stop This Shortcut**

   **End If**
3. **Get Dictionary from Input** — input: `File`.
4. **Get Dictionary Value** — `url` in `Dictionary`.
5. **Get Dictionary Value** — `token` in `Dictionary`.
6. **Format Date** — Date: `Current Date`; Date Format: `ISO 8601`;
   "Include ISO 8601 Time": **on**.
   The Transaction object exposes no date property (see below), and the
   automation runs within seconds of the tap, so the device's "now" — with
   its UTC offset, which is what the server uses for the wall-clock date —
   is the event time.
7. **Get Contents of URL**
   - URL: the `url` value (step 4) followed by the literal text
     `/api/v1/import/ingest-apple-wallet-event`.
   - Method: `POST`.
   - Headers: `Authorization` = the text `Bearer ` (trailing space)
     followed by the `token` value (step 5).
   - Request Body: `JSON`, fields:

     | Key | Type | Value |
     |---|---|---|
     | `account` | Text | `Shortcut Input` › **Card or Pass** |
     | `payee` | Text | `Shortcut Input` › **Merchant** |
     | `amount` | Text | `Shortcut Input` as **Currency Amount** › **Currency Amount** |
     | `currency` | Text | `Shortcut Input` as **Currency Amount** › **Currency Code** |
     | `occurredAt` | Text | `Formatted Date` (step 6) |
     | `type` | Text | `expense` |

   Every field is Text, including `amount`: the server parses the decimal
   itself and tolerates locale formatting; a Number field would let
   Shortcuts round or localize it.

Nothing after the request. The automation runs unattended; there is no one
to show a result to.

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
the object exposes no type property, so v1 sends `expense` for everything; a
refund imports as an expense with a positive amount and needs a hand fix.

## Same-named cards (per-card variant)

Two cards with the same display name are indistinguishable to the server,
because `account` is the card name. The user-side fix: duplicate
`econumo-wallet-v1` (right-click → Duplicate), rename it (`econumo-wallet-v1
Visa joint`), replace the `account` value with a typed literal, and point a
Transaction automation filtered to that one card at the copy. Nothing
changes server-side; the literal is just another external account name.

## Test on a device before shipping

The plists are generated from documented action formats, but the
Transaction property names, the coercion, and the ISO date output were
never exercised on a device by the build — this pass is what proves them.

1. Open both served `.shortcut` files on an iPhone in Safari (or AirDrop
   them). The "Add Shortcut" sheet must appear without a "cannot be opened
   because it is not signed" error. After adding, the names in the
   library are `econumo-setup-v1` and `econumo-wallet-v1` (iOS takes them
   from the file name, which is why the recipe names match the files).
2. In Safari on the phone, open Econumo → Settings → Import & export → Configure.
   Shortcuts should open, run `econumo-setup-v1`, and show the notification.
   Check `Files` → iCloud Drive → Shortcuts → `econumo-wallet.json`.
3. Create the automation: Shortcuts → Automation → `+` → **Transaction** →
   any card, any type → **Run Immediately** → `econumo-wallet-v1`.
4. Make an Apple Pay purchase and watch the event appear on the Econumo
   queue page or as an imported transaction. Check the stored event: card
   name and merchant filled, `amount` a bare number, `currency` a 3-letter
   code, `occurredAt` carrying the device's offset (e.g. `+02:00`).
5. Delete the JSON file and tap a card again: the "not configured"
   notification must appear and nothing must be posted.

Known things to confirm during this test (the spec lists them as device
tests): `Get File` works inside a background Transaction run, and
`Save File` on a phone with iCloud Drive turned off (fallback: the
on-device Shortcuts folder).
