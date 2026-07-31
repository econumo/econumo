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
    <img src="docs/screenshots/budget.png" alt="Econumo budget — envelope budgeting with folders, limits, and available amounts" width="800">
</p>

<details>
<summary><b>More screenshots</b> — transactions, adding a transaction, sharing with family</summary>
<br>
<table>
  <tr>
    <td align="center"><img src="docs/screenshots/transactions.png" alt="Transaction list of an account"><br><sub>Transactions</sub></td>
    <td align="center"><img src="docs/screenshots/add-transaction.png" alt="Add-transaction dialog with built-in calculator and tags"><br><sub>Adding a transaction</sub></td>
  </tr>
  <tr>
    <td align="center"><img src="docs/screenshots/shared-access.png" alt="Sharing budgets and accounts with family members"><br><sub>Manage money together</sub></td>
    <td align="center"><img src="docs/screenshots/mobile.png" alt="Mobile view of accounts" height="420"><br><sub>Works great on mobile</sub></td>
  </tr>
</table>
</details>

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
walkthrough.

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

### Contact

- For release announcements, please check [GitHub Releases](https://github.com/econumo/econumo/releases) or [Econumo Website](https://econumo.com/tags/release/).
- For questions, issue reporting, or advice, please use [GitHub Issues](https://github.com/econumo/econumo/issues).

---
> [!NOTE]
> Econumo is funded by our `GitHub Sponsors` and `Econumo` (cloud) subscribers.
>
> If you know someone who might [find Econumo useful](https://econumo.com/), we'd appreciate if you'd let them know.
