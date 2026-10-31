<p align="center">
    <picture>
        <img src="https://github.com/econumo/.github/raw/master/profile/econumo.png" width="194">
    </picture>
</p>

<p align="center">
    A getting started guide to self-hosting <a href="https://econumo.com/docs/edition" target="_blank">Econumo</a> — a personal finance & budgeting app
</p>

---

Econumo ships as a single, self-contained Go binary in a distroless Docker image.
It serves both the API and the web app, runs database migrations automatically on
boot, and works with SQLite (default) or PostgreSQL.

<p align="center">
    <img src="docs/screenshots/budget.png" alt="Econumo budget — income, savings and expenses for the month with planned, actual and available amounts" width="800">
</p>

> [!IMPORTANT]
> The Docker image is now published to the **GitHub Container Registry**:
> `ghcr.io/econumo/econumo`. The old Docker Hub image (`econumo/econumo-ce`)
> belongs to v0.x and is no longer updated — update your `docker-compose.yml`
> or pull references accordingly.

### Quick start

You'll need [Docker](https://docs.docker.com/engine/install/) with
[Compose](https://docs.docker.com/compose/install/). The app itself is
lightweight — it consumes up to 10 MB of RAM.

```console
$ git clone --single-branch https://github.com/econumo/econumo
$ cd econumo
$ cp .env.example .env
$ docker compose pull && docker compose up -d
```

Then visit `http://localhost:8181` and create the first user.

> [!NOTE]
> To build the image from source instead of pulling, run
> `docker compose up -d --build` (the `Dockerfile` is in
> [`deployment/docker/`](deployment/docker/Dockerfile)). Health is reported
> at `/health`.

### Configuration

Everything is configured through environment variables in `.env` —
[`.env.example`](.env.example) is the full, commented reference for every
setting (database, mail, currencies, CORS, logging). The defaults
work out of the box: SQLite storage and registration enabled; the only
variables most setups ever touch are `DATABASE_URL` (to switch to PostgreSQL)
and `MAILER_DSN` (to send password-recovery email).

CLI commands (create users, update currency rates, …) run through the binary
inside the container, e.g.:

```console
$ docker compose exec econumo /app/econumo user:create "Name" user@example.com password
```

### Run without Docker (single binary)

Prefer not to use Docker? Every release also ships self-contained Linux
binaries with the web UI embedded, runnable under systemd on a single host.
See [docs/run-without-docker.md](docs/run-without-docker.md) for the full
walkthrough. Hosts managed with [ship](https://github.com/dmtrkzntsv/ship) are
covered in [Deploy with ship](#deploy-with-ship).

## Deploy with ship

The maintainers' own hosts install and update the single binary with
[ship](https://github.com/dmtrkzntsv/ship), which pulls each build from a
private R2 bucket, verifies it, renders the hardened systemd unit
[`deploy/ship/econumo.service`](deploy/ship/econumo.service) and rolls a failed
update back. The ship ref is `econumo`.

**Publish.** The release workflow publishes every release (and the Publish Dev
workflow the `dev` channel); by hand:

```console
$ make ship-dist VERSION=v1.2.3 && make ship-publish VERSION=v1.2.3 CHANNELS=latest
$ make ship-dist && make ship-publish CHANNELS=dev
```

`ship-dist` builds the linux binaries and stages them with the unit template
and [`deploy/ship/econumo.env.example`](deploy/ship/econumo.env.example) in
`dist/ship/`; `ship-publish` uploads that folder (it needs
`SHIP_R2_ENDPOINT`, `SHIP_R2_ACCESS_KEY_ID`, `SHIP_R2_SECRET_ACCESS_KEY` or
ship's config file).

**Install.** Install ship once per host (with the bucket's read-only key in
`/etc/ship/config.toml`), then:

```console
$ sudo ship install econumo --version latest
$ sudoedit /etc/econumo/econumo.env      # seeded from the env example
$ sudo ship start econumo
```

The binary lands in `/opt/econumo/econumo` and the data in `/var/lib/econumo`
(the default `DATABASE_URL=sqlite://db.sqlite` is relative to it).
`--service NAME` installs another instance under its own name, e.g.
`sudo ship install econumo --service econumo-beta --version dev`; pass the same
`--service` to every command below.

**Operate.**

```console
$ sudo ship update econumo                # newest build of its channel
$ sudo ship rollback econumo
$ sudo ship logs econumo
$ sudo ship status --service econumo
$ sudo ship run econumo -- user:create "Name" user@example.com password
```

**Moving an existing host.** A host set up by the former `econumo-deploy.sh`
moves over once, with a minute of downtime, following ship's
[Moving a hand-installed service to ship](https://github.com/dmtrkzntsv/ship#moving-a-hand-installed-service-to-ship).
Keep the service name and the data directory is reused as it is: stop and
disable the old unit, move its unit file, `/etc/S` and `/opt/S` aside, run
`ship install`, then copy the old env file into place. The values for service
`S` (`econumo`, or e.g. `econumo-beta`):

| | old (`econumo-deploy.sh`) | ship |
|---|---|---|
| unit | `/etc/systemd/system/S.service` | same path, rendered by ship |
| binary | `/opt/S/econumo` | `/opt/S/econumo` |
| env file | `/etc/S/env` | `/etc/S/econumo.env` (copy from `/etc/S.pre-ship/env`) |
| user | `S` | `S` |
| data dir | `/var/lib/S` | `/var/lib/S`, reused as is |
| key files | the Sign in with Apple `.p8`, if `ECONUMO_OAUTH_APPLE_PRIVATE_KEY_FILE` points into `/etc/S` | copy it back from `/etc/S.pre-ship/` and `chgrp S` it |

### Localization

All translations live in [`locales/`](locales/) — one JSON catalogue per
language, shared by the backend and the web app and managed right in the
repository (no external translation platform). To contribute a language, copy
[`locales/en.json`](locales/en.json), translate the values, and open a pull
request — the test suite verifies key and placeholder parity between
catalogues automatically.

### Upgrading from v0.x (PHP)

v1.x is a full rewrite — the PHP backend became the Go binary and the Vue.js
frontend became a React app. The result: memory consumption dropped from
~200 MB to ~10 MB, the app is much faster, and the new UI is a big step up.
Your database is reused in place — accounts, passwords, and data keep
working. See the
**[migration guide](docs/migration-v0-to-v1.md)** for the step-by-step
walkthrough (backup, new image, `.env` mapping, and the gotchas).

### Next steps

Everything else — self-hosting (multi-currency, backups, CLI commands,
debugging), the API, MCP, and the user guide — lives in the
**[Econumo documentation](https://econumo.com/docs)**.

To connect Claude or Codex to your instance over MCP with a browser sign-in,
see **[docs/mcp-setup.md](docs/mcp-setup.md)**.

### Contact

- For release announcements, please check [GitHub Releases](https://github.com/econumo/econumo/releases) or [Econumo Website](https://econumo.com/tags/release/).
- For questions, issue reporting, or advice, please use [GitHub Issues](https://github.com/econumo/econumo/issues).

---
> [!NOTE]
> Econumo is funded by our `GitHub Sponsors` and `Econumo` (cloud) subscribers.
>
> If you know someone who might [find Econumo useful](https://econumo.com/), we'd appreciate if you'd let them know.
