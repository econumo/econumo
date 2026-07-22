# Run without Docker (single binary)

Every release ships self-contained Linux binaries alongside the Docker image.
The web UI is embedded in the binary, so a single file is the whole app — no
separate frontend to serve, no runtime assets to mount.

## Download and verify

Grab the binary for your architecture plus `SHA256SUMS` from the
[latest release](https://github.com/econumo/econumo/releases/latest) and verify
it:

```console
$ curl -LO https://github.com/econumo/econumo/releases/latest/download/econumo-linux-amd64
$ curl -LO https://github.com/econumo/econumo/releases/latest/download/SHA256SUMS
$ sha256sum --check --ignore-missing SHA256SUMS
```

Binaries are published for `linux/amd64` and `linux/arm64`.

## Install as a systemd service

A reference unit lives in
[`deployment/systemd/econumo.service`](../deployment/systemd/econumo.service):

```console
$ sudo useradd --system --home-dir /var/lib/econumo --shell /usr/sbin/nologin econumo
$ sudo mkdir -p /opt/econumo /var/lib/econumo /etc/econumo
$ sudo chown econumo:econumo /var/lib/econumo
$ sudo install -m 0755 econumo-linux-amd64 /opt/econumo/econumo
$ sudoedit /etc/econumo/env
$ sudo cp deployment/systemd/econumo.service /etc/systemd/system/
$ sudo systemctl daemon-reload && sudo systemctl enable --now econumo
```

A minimal `/etc/econumo/env` (all other settings from
[`.env.example`](../.env.example) work here too):

```
DATABASE_URL=sqlite:///var/lib/econumo/db.sqlite
PORT=8181
```

## Configuration

Everything is configured through environment variables — see
[`.env.example`](../.env.example) for the full, commented reference. The web UI
is embedded, so there is no directory to point at; instance-specific frontend
values (analytics, billing URL, liltag config, the version label, …) are set
through `ECONUMO_*` variables and merged into the served config at runtime.

## Upgrades

Replace `/opt/econumo/econumo` with the new release binary and
`sudo systemctl restart econumo` — database migrations run on boot, exactly as
in the Docker image. `/opt/econumo/econumo version` prints the installed
version.

## Management commands

CLI commands (create users, update currency rates, …) need the same environment
as the service:

```console
$ sudo -u econumo sh -c 'set -a; . /etc/econumo/env; exec /opt/econumo/econumo user:create "Name" user@example.com password'
```

## Backups

Back up `/var/lib/econumo` — the SQLite database is the only state.
