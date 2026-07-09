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
setting (database, mail, currencies, CORS, JWT keys, logging). The defaults
work out of the box: SQLite storage and registration enabled; the only
variables most setups ever touch are `DATABASE_URL` (to switch to PostgreSQL)
and `MAILER_DSN` (to send password-recovery email).

All mutable state lives in two Docker volumes — `db` (the SQLite database) and
`jwt` (the keypair) — so your data and logins survive container recreation.
Back both up.

CLI commands (create users, update currency rates, …) run through the binary
inside the container, e.g.:

```console
$ docker compose exec econumo /app/econumo user:create "Name" user@example.com password
```

### Upgrading from v0.x (PHP)

v1.x is a full rewrite — the PHP backend became the Go binary and the Vue.js
frontend became a React app — but it reuses your database in place: accounts,
passwords, and data keep working. See the
**[migration guide](docs/migration-v0-to-v1.md)** for the step-by-step
walkthrough (backup, new image, `.env` mapping, and the gotchas).

### Next steps

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
