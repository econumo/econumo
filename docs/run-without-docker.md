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

Create the user and directories, install the binary and write the env file:

```console
$ sudo useradd --system --home-dir /var/lib/econumo --shell /usr/sbin/nologin econumo
$ sudo mkdir -p /opt/econumo /var/lib/econumo /etc/econumo
$ sudo chown econumo:econumo /var/lib/econumo
$ sudo install -m 0755 econumo-linux-amd64 /opt/econumo/econumo
$ sudoedit /etc/econumo/env
```

A minimal `/etc/econumo/env` (all other settings from
[`.env.example`](../.env.example) work here too):

```
DATABASE_URL=sqlite:///var/lib/econumo/db.sqlite
PORT=8181
```

Then save this unit as `/etc/systemd/system/econumo.service` and start it with
`sudo systemctl daemon-reload && sudo systemctl enable --now econumo`:

```ini
[Unit]
Description=Econumo personal finance server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=econumo
Group=econumo
WorkingDirectory=/var/lib/econumo
EnvironmentFile=/etc/econumo/env
ExecStart=/opt/econumo/econumo serve
Restart=on-failure
RestartSec=5

# Hardening: the process only ever writes its database directory.
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/econumo

[Install]
WantedBy=multi-user.target
```

[`deployment/systemd/econumo.service`](../deployment/systemd/econumo.service)
is the same unit as a template for [ship](#hosts-managed-with-ship); its
`{{...}}` placeholders are filled in by ship, so do not copy it as is.

## Hosts managed with ship

The Econumo maintainers' own hosts install and update the binary with
[ship](https://github.com/dmtrkzntsv/ship), which pulls each release from a
private R2 bucket (it needs that bucket's read-only credentials in
`/etc/ship/config.toml`), verifies it, installs the hardened unit above and
rolls a failed update back. Every release publishes the binaries, the unit
template and an env example
([`deployment/systemd/econumo.env.example`](../deployment/systemd/econumo.env.example)).

Install ship once per host, then install Econumo:

```console
$ curl -fsSL https://github.com/dmtrkzntsv/ship/releases/latest/download/install.sh | sudo sh
$ sudo ship install econumo --version latest
$ sudoedit /etc/econumo/econumo.env      # created from the env example
$ sudo ship start econumo
```

The binary lands in `/opt/econumo/econumo`, the env file in
`/etc/econumo/econumo.env`, the data in `/var/lib/econumo`; `--service NAME`
installs a second instance (e.g. `econumo-beta` on `--version dev`) under its
own name. Day to day:

```console
$ sudo ship update econumo                # to the newest build of its channel
$ sudo ship rollback econumo
$ sudo ship logs econumo
$ sudo ship run econumo -- user:create "Name" user@example.com password
```

A host set up by the former `econumo-deploy.sh` script (binary
`/opt/econumo/econumo`, env `/etc/econumo/env`, unit
`/etc/systemd/system/econumo.service`) is taken over in place, keeping its
user and data; its env file moves to `/etc/econumo/econumo.env`:

```console
$ sudo ship adopt econumo --service econumo --version latest
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
