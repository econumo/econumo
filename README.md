<p align="center">
    <picture>
        <img src="https://github.com/econumo/.github/raw/master/profile/econumo.png" width="194">
    </picture>
</p>

<p align="center">
    A getting started guide to self-hosting <a href="https://econumo.com/docs/edition" target="_blank">Econumo</a>
</p>

---

### Prerequisites

- **[Docker](https://docs.docker.com/engine/install/)** and **[Docker Compose](https://docs.docker.com/compose/install/)** must be installed on your machine.
- At least **256 MB of RAM** is recommended.

### Quick start

1. Clone this repository:

    ```console
    $ git clone --single-branch https://github.com/econumo/econumo
    Cloning into 'econumo'...
    remote: Enumerating objects: 13, done.
    remote: Counting objects: 100% (10/10), done.
    remote: Compressing objects: 100% (9/9), done.
    remote: Total 13 (delta 0), reused 7 (delta 0), pack-reused 3 (from 1)
    Receiving objects: 100% (13/13), done.

    $ cd econumo

    $ ls -1 docker-compose.yml .env.example
    docker-compose.yml
    .env.example
    ```

2. Create and configure your [environment](https://docs.docker.com/compose/environment-variables/) file:

    ```console
    $ cp .env.example .env
    ```

3. Pull the published image and start the services:

    ```console
    $ docker compose pull
    $ docker compose up -d
    ```

    > To build the image from source instead of pulling, run `docker compose up -d --build`.

4. Visit your instance at `http://localhost:8181` and create the first user.

> [!NOTE]
> Econumo ships as a single, self-contained Go binary (a distroless image), so it
> starts in seconds and runs migrations automatically on boot. The image is
> published to `ghcr.io/econumo/econumo`; the `Dockerfile` lives in this
> repository under [`deployment/docker/`](deployment/docker/Dockerfile).
> Health is reported at `/health`.


### Upgrading from v0.x (PHP) to v1.x

v1.x replaces the PHP/Symfony backend with a single Go binary, but it reuses your
existing database **in place** — there is no export/import. v0.x already ran on
SQLite or PostgreSQL (the same two engines v1.x supports), the schema is carried
forward unchanged, and the Go binary detects your old Doctrine migrations on boot
and applies only the genuinely new ones. Your accounts, passwords, and data all
keep working.

> [!IMPORTANT]
> The upgrade reuses your database in place and runs migrations on first boot.
> **Always back up your database first** (step 1) — it is your only rollback if
> anything goes wrong.

**1. Back up your database.** Do this before changing anything, while the old
stack is stopped.

- **SQLite** — copy the database file (and ideally the rest of `var/`):

    ```console
    $ docker compose down
    $ cp var/db/db.sqlite "var/db/db.sqlite.bak-$(date +%Y%m%d)"
    ```

- **PostgreSQL** — take a `pg_dump` you can restore from:

    ```console
    $ pg_dump "$DATABASE_URL" > "econumo-backup-$(date +%Y%m%d).sql"
    ```

  Restore later with `psql "$DATABASE_URL" < econumo-backup-YYYYMMDD.sql` if needed.

**2. Point v1.x at your existing data.**

- **SQLite** — your v0.x database lived at `var/db/db.sqlite`. Place that file
  where the new container reads it (`/app/var/db/db.sqlite` inside the `db`
  volume; `DATABASE_URL=sqlite:///app/var/db/db.sqlite` is the default).

    The v1.x container runs as the non-root user `nonroot` (UID/GID **65532**),
    while a carried-over v0.x file is usually owned by root. SQLite must also
    write a `-wal`/`-journal` file *next to* the database, so the **directory**
    needs to be writable too — otherwise boot fails with
    `attempt to write a readonly database (8)`. Re-own the whole `db` volume
    (run on the host, with the container stopped):

    ```console
    $ sudo chown -R 65532:65532 \
        "$(docker volume inspect <project>_db --format '{{ .Mountpoint }}')"
    ```

    Replace `<project>` with your compose project name (the directory you run
    `docker compose` from, e.g. `econumo_db`). Do the same for the `jwt` volume
    if you carried one over. `ls -lan` should then show both the directory (`.`)
    and `db.sqlite` owned by `65532 65532`.

    Alternatively, do it manually step by step — list the volumes, resolve the
    host path, then `chown` it directly:

    ```console
    $ docker volume ls
    DRIVER    VOLUME NAME
    local     econumo_db
    local     econumo_jwt

    $ docker volume inspect econumo_db --format '{{ .Mountpoint }}'
    /var/lib/docker/volumes/econumo_db/_data

    $ cd /var/lib/docker/volumes/econumo_db/_data
    $ ls -lan                      # check current ownership
    $ sudo chown -R 65532:65532 .  # the trailing "." re-owns the directory itself
    $ ls -lan                      # confirm "." and db.sqlite show 65532 65532
    ```

    Use `.` (not `./*`) so the directory itself is re-owned, not just its
    contents — SQLite needs to create its journal file inside that directory.
- **PostgreSQL** — keep your database where it is and set `DATABASE_URL` to it,
  e.g. `DATABASE_URL=postgres://econumo:econumo@your-db-host:5432/econumo`. The
  Postgres service is no longer part of the bundled compose file; manage it
  yourself or migrate to SQLite.

On first boot v1.x runs migrations automatically and recognizes your already-applied
v0.x migrations, so it won't try to re-create the schema.

**3. Rewrite your `.env`.** The Symfony variables are gone; start from the new
[`.env.example`](.env.example). The mapping:

| v0.x (PHP) | v1.x (Go) | Notes |
|---|---|---|
| `DATABASE_DRIVER` + `SQLITE_DATABASE_URL` / `POSTGRES_DATABASE_URL` | `DATABASE_URL` | Set the DSN directly; the scheme picks the engine. |
| `APP_ENV=dev` | `ECONUMO_DEBUG=true` | Only the dev stack-trace behavior carries over. |
| `JWT_PASSPHRASE` | `ECONUMO_JWT_PASSPHRASE` | Optional; keys auto-generate (see below). |
| `CORS_ALLOW_ORIGIN` | `ECONUMO_CORS_ALLOW_ORIGIN` | Empty now means same-domain only. |
| `ECONUMO_SQLITE_BUSY_TIMEOUT` | `SQLITE_BUSY_TIMEOUT` | Renamed. |
| `ECONUMO_FROM_EMAIL` / `ECONUMO_REPLY_TO_EMAIL` + mail transport | `MAILER_DSN` | Consolidated — see the gotcha below. |
| `ECONUMO_CURRENCY_BASE`, `ECONUMO_ALLOW_REGISTRATION`, `OPEN_EXCHANGE_RATES_TOKEN` | _unchanged_ | |
| `ECONUMO_DATA_SALT` | _deprecated_ | Same name, but v1.x runs salt-free — migrate once if it was set (see below). |
| `APP_SECRET`, `ECONUMO_BASE_URL`, `ECONUMO_SYSTEM_API_KEY`, `ECONUMO_CONNECT_USERS`, `MESSENGER_TRANSPORT_DSN`, `LOCK_DSN`, New Relic / Qase vars | _removed_ | Symfony-only; drop them. |

**Watch out for these:**

- **`MAILER_DSN` changed meaning.** In v0.x it was a Symfony Mailer DSN
  (`null://null`, `mailjet+api://…`). In v1.x the scheme is `console://` (the
  default — prints to stdout) or `resend://<api_key>?from=…&reply_to=…`. A
  leftover v0.x value will **fail at boot**, so clear it or set the new form.
- **`ECONUMO_DATA_SALT`** is deprecated, and v1.x runs **salt-free** — it no
  longer decrypts salted data. If your v0.x instance never set it (or set it
  empty), do nothing. **If it was set, you must migrate the data once**, or those
  users can't log in (their emails are unreadable and identifiers no longer match).
  Back up your database first, then:

    1. In `.env`, set `ECONUMO_DATA_SALT` to your **old** salt (so the migration
       can decrypt), and start v1.x: `docker compose up -d`.
    2. Run the one-off migration (the distroless image has no shell, so call the
       binary directly — decryption is one-way, hence the backup):

        ```console
        $ docker compose exec econumo /app/econumo data:remove-salt
        ```

       It decrypts every email to plaintext, re-derives identifiers without the
       salt, refuses to run on an empty salt, and is idempotent (safe to re-run).
    3. Remove/comment `ECONUMO_DATA_SALT` in `.env` and restart:
       `docker compose up -d`. The boot-time `ECONUMO_DATA_SALT is set` warning
       clearing confirms you're done.

**4. Replace your `docker-compose.yml`** with the v1.x single-service stack from
this repo (one `econumo` service, the `db` + `jwt` volumes, port `8181:80`). Then:

```console
$ docker compose pull && docker compose up -d
```

**Note:** v1.x generates a fresh JWT keypair on first boot, so existing login
tokens are invalidated and everyone signs in again once — passwords are unchanged.


### Next steps

After installation, you may need to complete additional configuration. Please refer to the following guides:

- [How to configure multi-currency support](https://econumo.com/docs/self-hosting/multi-currency/) (Econumo comes preloaded with **USD** only).
- [How to configure backups](https://econumo.com/docs/self-hosting/backups/).
- [Useful CLI commands](https://econumo.com/docs/self-hosting/cli-commands/).
- [How to debug Econumo](https://econumo.com/docs/self-hosting/debug/).
- [Econumo API](https://econumo.com/docs/api/).
- [User Guide](https://econumo.com/docs/user-guide/).

For more information please see our [documentation](https://econumo.com/docs/).

### Contact

- For release announcements, please check [GitHub Releases](https://github.com/econumo/econumo/releases) or [Econumo Website](https://econumo.com/tags/release/).
- For questions, issue reporting, or advice, please use [GitHub Issues](https://github.com/econumo/econumo/issues).


---
> [!NOTE]
> Econumo is funded by our `GitHub Sponsors` and `Econumo` (cloud) subscribers.
>
> If you know someone who might [find Econumo useful](https://econumo.com/), we'd appreciate if you'd let them know.
