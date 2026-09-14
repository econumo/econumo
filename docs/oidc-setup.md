# Sign in with Google / Apple / SSO

Econumo can offer "Sign in with…" buttons on the login and registration
pages, in addition to (never instead of) email + password. Each provider is
an independent, optional slot configured entirely through environment
variables — there is no admin UI for this. All three slots can be enabled at
once; a self-hosted instance typically enables one.

Every slot requires `ECONUMO_URL` to be set first: it is this instance's
public URL, and every callback URI is derived from it —
`<ECONUMO_URL>/api/v1/oauth/callback-<google|apple|oidc>` — rather than
configured separately. That derived URL is the one thing you register with
each provider below. A slot is all-or-nothing: setting some but not all of
its variables fails the server at boot, naming the missing one.

An issuer whose ID tokens never carry an `email_verified` claim will reject
every sign-in with "The sign-in provider has not verified this email
address." unless you set `ECONUMO_OIDC_TRUST_EMAIL=true` for that slot (see
the Authentik and Cloudflare Access sections below for when that applies).

## Google

1. In the [Google Cloud console](https://console.cloud.google.com/), open
   **APIs & Services → Credentials** for your project (create one if you
   don't have one yet).
2. If prompted, configure the **OAuth consent screen** first (External user
   type is fine for a self-hosted instance; add your own email as a test
   user if the app stays in "Testing" mode).
3. **Create Credentials → OAuth client ID**, application type **Web
   application**.
4. Under **Authorized redirect URIs**, add the exact callback URL (see
   below).
5. Save, then copy the generated **Client ID** and **Client secret** into:

   ```
   ECONUMO_OAUTH_GOOGLE_CLIENT_ID=<client id>
   ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET=<client secret>
   ```

Google is a fixed issuer (`https://accounts.google.com`) — there is no issuer
URL to configure. Google's email claim is used only when Google reports it as
verified (`email_verified`); an unverified Google address is rejected with
"The sign-in provider has not verified this email address."

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-google`

## Apple

Apple's flow needs a *Services ID* (which is the OAuth client id Econumo
uses), plus a private key downloaded once from the developer portal — Apple
never lets you download it again, so save it somewhere safe immediately.

1. In [Apple's developer portal](https://developer.apple.com/account/resources/identifiers/list),
   note your **Team ID** (top right of the membership page) — this becomes
   `ECONUMO_OAUTH_APPLE_TEAM_ID`.
2. Under **Identifiers**, register an **App ID** if you don't already have
   one for this app, with **Sign in with Apple** enabled as a capability.
3. Under **Identifiers**, register a new **Services ID** (type "Services
   IDs"). This identifier string (e.g. `com.example.econumo.signin`) is
   `ECONUMO_OAUTH_APPLE_CLIENT_ID`.
4. Edit the new Services ID, enable **Sign in with Apple**, click
   **Configure**, and add your primary App ID as the associated app. Under
   **Website URLs**, set the **Return URL** to the exact callback URL below
   (Apple's flow posts back via a form, but the callback route is still what
   you register).
5. Under **Keys**, register a new key with **Sign in with Apple** enabled,
   associated with the App ID from step 2. Download the `.p8` file — this is
   a one-time download. Note the **Key ID** shown on the key's page —
   this becomes `ECONUMO_OAUTH_APPLE_KEY_ID`.
6. The `.p8` file is a PEM-encoded private key with real newlines, but a
   `.env` file is single-line per variable. Replace every newline in the
   file's contents with the two characters `\n` (backslash, n) before
   pasting it in — Econumo unescapes literal `\n` back into real newlines
   when it reads the variable. For example, a key that looks like:

   ```
   -----BEGIN PRIVATE KEY-----
   MIGTAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBHkw...
   -----END PRIVATE KEY-----
   ```

   becomes one line:

   ```
   ECONUMO_OAUTH_APPLE_PRIVATE_KEY=-----BEGIN PRIVATE KEY-----\nMIGTAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBHkw...\n-----END PRIVATE KEY-----\n
   ```

7. Put it all together:

   ```
   ECONUMO_OAUTH_APPLE_CLIENT_ID=com.example.econumo.signin
   ECONUMO_OAUTH_APPLE_TEAM_ID=<team id>
   ECONUMO_OAUTH_APPLE_KEY_ID=<key id>
   ECONUMO_OAUTH_APPLE_PRIVATE_KEY=<the one-line key from step 6>
   ```

Apple is a fixed issuer (`https://appleid.apple.com`) and, like Google,
its email claim is always treated as verified. Apple only sends the user's
name on the very first sign-in — Econumo captures it then, so if the
first attempt fails for another reason, a later successful sign-in may not
have a name to use (it falls back to the email's local part).

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-apple`

## A generic OpenID Connect provider (Authentik example)

The third slot works with any standards-compliant OpenID Connect provider —
Authentik, Keycloak, Zitadel, Okta, Auth0, and so on. This section walks
through [Authentik](https://goauthentik.io/) as the concrete example; the
shape is the same everywhere.

1. In Authentik, create a new **OAuth2/OpenID Provider** (**Applications →
   Providers → Create**).
2. Set **Redirect URIs** to the exact callback URL below.
3. Under **Advanced protocol settings**, set the **Signing Key** to an RSA
   key (Econumo verifies RS256- or ES256-signed ID tokens).
4. Note the provider's **Client ID** and **Client Secret**, and the
   **OpenID Configuration URL** it displays — the issuer URL is that same
   value with the trailing `/.well-known/openid-configuration` removed, e.g.
   `https://auth.example.com/application/o/econumo/`.
5. Create an **Application** (**Applications → Applications → Create**) and
   attach it to the provider from step 1.
6. If you want the "sign out here too" logout redirect to work (Econumo's
   RP-initiated logout, web only), add a **post-logout redirect URI** on the
   provider matching `<ECONUMO_URL>/login`.
7. Configure:

   ```
   ECONUMO_OIDC_ISSUER_URL=https://auth.example.com/application/o/econumo/
   ECONUMO_OIDC_CLIENT_ID=<client id>
   ECONUMO_OIDC_CLIENT_SECRET=<client secret>
   ECONUMO_OIDC_NAME=Authentik
   ECONUMO_OIDC_SCOPES=openid,profile,email
   ```

   Leave `ECONUMO_OIDC_TRUST_EMAIL` unset (`false`): Authentik's ID tokens
   carry `email_verified`, so Econumo can trust it directly.

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-oidc`
**Post-logout redirect URI (optional, for RP-initiated logout):**
`<ECONUMO_URL>/login`

## Cloudflare Access for SaaS

[Cloudflare Access for SaaS](https://developers.cloudflare.com/cloudflare-one/applications/configure-apps/saas-apps/generic-oidc-saas-app/)
can front Econumo as a generic OIDC identity provider, so your whole
Cloudflare Access policy (any identity provider you've already wired into
Access, any Access group) becomes an Econumo sign-in method with no separate
user database.

1. In the Cloudflare Zero Trust dashboard, go to **Access → Applications →
   Add an application → SaaS**, and choose **OIDC** as the SaaS application
   type.
2. Set the **Redirect URL(s)** to the exact callback URL below.
3. Save. Cloudflare shows you a **Client ID**, **Client Secret**, and a
   **Issuer URL** for the application. The issuer URL has the shape:

   ```
   https://<your-team-name>.cloudflareaccess.com/cdn-cgi/access/sso/oidc/<client-id>
   ```

4. Configure:

   ```
   ECONUMO_OIDC_ISSUER_URL=https://<your-team-name>.cloudflareaccess.com/cdn-cgi/access/sso/oidc/<client-id>
   ECONUMO_OIDC_CLIENT_ID=<client id>
   ECONUMO_OIDC_CLIENT_SECRET=<client secret>
   ECONUMO_OIDC_NAME=SSO
   ECONUMO_OIDC_TRUST_EMAIL=true
   ```

   **Cloudflare Access's ID tokens do not include an `email_verified`
   claim**, so `ECONUMO_OIDC_TRUST_EMAIL` must be set to `true` for this
   slot — leaving it unset means every sign-in through Access is rejected
   with "The sign-in provider has not verified this email address." Only
   set this if you trust that Access itself only lets verified identities
   through (true for most upstream identity providers wired into Access).

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-oidc`

## How accounts are matched

- **Signing in with a provider whose email matches an existing account that
  has no password** links the two automatically — that account was itself
  created through a provider, so both sides have proven the same address.
  You're signed in to the existing account and the new provider appears under
  Settings → Profile → Linked accounts; the account owner gets an email saying
  so, because gaining a sign-in method unasked should be noticeable.
- **If the matching account HAS a password, the sign-in is refused** with "An
  account with this email address already exists. Sign in with your password
  (or reset it), then link this provider from Settings." Econumo will not merge
  a provider identity into an account whose owner has not authenticated:
  registration does not always verify email addresses, so a password on an
  account is no evidence that its holder owns the address — someone could have
  registered yours before you did. Signing in once with the password (use
  "Forgot password" if you never set one; the code goes to that mailbox) and
  then linking from Settings is the safe path, and only has to be done once.
- **A password reset is a full account reclaim.** Completing one is the only
  way to prove you control the address, so it also signs out every session,
  revokes every personal access token, cancels anything still pending on the
  account (an unfinished provider sign-in, a requested email change), and
  unlinks any sign-in method whose
  provider reports a *different* email address — otherwise someone who had
  registered your address first could keep a provider account linked to it and
  walk straight back in. A provider that reports the same address as the
  account survives the reset (only the mailbox owner could have linked it),
  which is why setting a password on a provider-created account does not
  disturb it. If you had linked a provider under a different address, link it
  again from Settings afterwards.
- **Linking a provider from Settings is finished by the browser that started
  it.** The provider's answer carries nothing that identifies you, so Econumo
  parks the result and only writes the link when the browser that began it
  comes back signed in and presenting its one-flow secret. A "link this
  account" URL someone else sends you therefore cannot attach your provider
  account to theirs.
- **A sign-in, too, can only be completed by the browser that started it.** The
  "Continue with ..." button receives a one-flow secret from the server and
  keeps it locally; the browser must present it together with the one-time
  handoff code to receive a session. Starting a sign-in in one browser and
  finishing it in another therefore fails with "The sign-in attempt expired or
  was already used" — as does a sign-in link someone else hands you.
- **Changing `ECONUMO_OIDC_ISSUER_URL` later is safe.** A linked identity
  records the issuer it came from, so a user at the new issuer can never be
  mistaken for a user at the old one, even if the two hand out the same
  subject id. Existing users are re-matched by their verified email on the next
  sign-in and their linked account moves to the new issuer by itself.
- **An email the provider has not verified is always rejected**, even if it
  would otherwise match an existing account. This is what
  `ECONUMO_OIDC_TRUST_EMAIL` is for: some providers (see Cloudflare Access
  above) never send the verification claim at all, so you tell Econumo to
  trust them explicitly.
- **Signing in for the first time with no matching account** creates a new
  Econumo account automatically, as long as registration is enabled
  (`ECONUMO_ALLOW_REGISTRATION=true`) — otherwise you'll see "Registration is
  disabled on this server. Sign in with an existing account first." An
  account created this way has no password: you can keep using the provider
  to sign in indefinitely, or add a password later.
- **Setting a password**: go to Settings → Profile and use the "Set a
  password" option (in place of "Change password" for a passwordless
  account) — it sends a reset code to your email the same way "Forgot
  password" does. Once set, the account behaves like any password account:
  you can sign in either way, and you can unlink every provider (Econumo
  always refuses to remove your *last* remaining sign-in method while you
  have no password, so you can never lock yourself out).
- **If the provider later reports a different email for the same
  identity** (you changed your email at the IdP), Econumo updates your
  Econumo email to match automatically, but only while the account is
  still passwordless, has exactly one linked provider, and the new address
  isn't already used by another Econumo account. Add a password or a second
  linked provider and this stops happening — after that, changing your
  email goes through Settings → Profile like any other account.
