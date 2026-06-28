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

    $ cd econumo/deployment/docker-compose

    $ ls -1
    docker-compose.yml
    .env.example
    ```

2. Create and configure your [environment](https://docs.docker.com/compose/environment-variables/) file:

    ```console
    $ cp .env.example .env
    ```

3. Start the services with Docker Compose:

    ```console
    $ docker-compose up -d
    ```

4. Visit your instance at `http://localhost:8181` and create the first user.

> [!NOTE]
> Econumo ships as a single, self-contained Go binary (a distroless image), so it
> starts in seconds and runs migrations automatically on boot. The image is
> published to `ghcr.io/econumo/econumo`; the `Dockerfile` lives in this
> repository under [`deployment/docker/go/`](deployment/docker/go/Dockerfile).
> Health is reported at `/_/health-check`.


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
