# GoLive Bare Git Deployment Guide

Target server: `root@154.36.185.85`

Deployment works by pushing from the local machine to a bare repository on the server.
The server's `post-receive` hook checks out the code, builds the frontend, and restarts Compose services automatically.

## 1. Prepare the server

Recommended system: Ubuntu 22.04 LTS.

Expose only the required ports to the public internet:

```text
80    Web HTTP
443   Web HTTPS
1935  OBS RTMP publishing
```

Do not expose internal ports for MySQL, Redis, Kafka (optional), Prometheus, Grafana,
Jaeger, pprof, or other internal services to the public internet.

Install the base components:

```bash
apt update
apt install -y git ca-certificates curl docker.io docker-compose-plugin
systemctl enable --now docker
```

For networks in mainland China, configuring a Docker registry mirror is recommended. Then run:

```bash
systemctl restart docker
```

## 2. Create a bare repository

```bash
mkdir -p /srv/git /srv/golive/app
git init --bare /srv/git/golive.git
```

Install the deployment hook:

```bash
scp scripts/post-receive.golive.example root@154.36.185.85:/tmp/post-receive
ssh root@154.36.185.85
cp /tmp/post-receive /srv/git/golive.git/hooks/post-receive
chmod +x /srv/git/golive.git/hooks/post-receive
```

To update the hook later, run the following once the server working directory exists:

```bash
cp /srv/golive/app/scripts/post-receive.golive.example /srv/git/golive.git/hooks/post-receive
chmod +x /srv/git/golive.git/hooks/post-receive
```

A push always runs the hook that is already installed, so a push that changes the hook still
deploys with the old one. To use a new hook for that same deploy, install it from your local
checkout before pushing:

```bash
scp scripts/post-receive.golive.example root@154.36.185.85:/srv/git/golive.git/hooks/post-receive
ssh root@154.36.185.85 chmod +x /srv/git/golive.git/hooks/post-receive
```

## 3. Configure the server (`deploy/.env` and `deploy/secrets`)

Both live in `/srv/golive/app/golive-backend/deploy/` and are not in git; the hook's checkout
leaves them alone, so you can create them before the first push
(`mkdir -p /srv/golive/app/golive-backend/deploy/secrets`).

- `deploy/.env`: copy `golive-backend/deploy/.env.example` and fill in every value. Compose
  refuses to start while a required variable (`${VAR:?…}` in `docker-compose.yml`) is empty:
  the MySQL passwords and DSN, `GOLIVE_REDIS_PASSWORD`, the Grafana admin, `GW_CSRF_SECRET`,
  `GOLIVE_INTERNAL_TOKEN` and `ROOMSVC_LIVE_STREAM_KEY_SECRET`. Generate secrets with
  `openssl rand -hex 32`, and avoid `$` in values because Compose expands it.
- `deploy/secrets/`: the JWT key files (`jwt_private.pem`, `jwt_public.pem`). They can stay
  owned by root with mode `0600` (see section 5).
- Optional: `COMPOSE_PROFILES=kafka` in `deploy/.env` starts the Kafka broker (section 5).

## 4. Add the remote locally and deploy

The current local branch is `master`; the recommended remote name is `prod`:

```bash
git remote add prod ssh://root@154.36.185.85/srv/git/golive.git
git push prod master
```

If the remote already exists:

```bash
git remote set-url prod ssh://root@154.36.185.85/srv/git/golive.git
git push prod master
```

`post-receive` automatically performs these steps:

```text
Check out the code into /srv/golive/app
Install dependencies and build golive-web/dist using node:20-alpine
Build the service images (docker compose build)
Remove the golive-kafka container if the kafka profile is not enabled
Start the stack: docker compose up -d --remove-orphans, giving up after UP_TIMEOUT (20m)
  - init-permissions runs and exits first (section 5)
  - each service starts once the services it depends on are healthy
Check single-file configuration mounts for drift in nginx / srs and force-recreate containers when needed
Print docker compose ps -a
```

`docker compose up` waits for health checks without a limit of its own: a service that keeps
crashing at startup never turns healthy or unhealthy, so without `UP_TIMEOUT` the push would
hang. When the limit is hit, or a dependency is reported unhealthy, the hook prints
`docker compose ps -a` and exits non-zero; check that service's logs.

> Edge containers (nginx, srs) mount configuration files such as
> `nginx.https.conf` and `srs.conf` using single-file bind mounts. Docker binds each mount to an inode at container creation.
> `git checkout -f` replaces files atomically, creating new inodes, so long-running containers may keep reading old
> files. `docker compose up -d` does not recreate containers when only a mounted file's content changes, and `nginx -s reload`
> cannot fix this because the mount still references the old inode. At the end of deployment, the hook therefore compares
> content hashes of host configuration files with the files actually mounted inside containers. Only when drift is detected does it run
> `docker compose up -d --force-recreate --no-deps <service>` for the affected service, ensuring configuration updates take effect.
> Note: recreating srs after a change to `srs.conf` briefly interrupts live stream publishing.

The deployment hook accepts `main` and `master` by default. Override these server variables as needed:

```bash
DEPLOY_BRANCH=main
FALLBACK_BRANCH=master
WORK_TREE=/srv/golive/app
GIT_DIR=/srv/git/golive.git
UP_TIMEOUT=20m
```

Every deploy may change the database schema (the services migrate it when they start), so
back up MySQL and Redis first; see [`deploy-runbook.md`](deploy-runbook.md).

## 5. Containers: runtime user, permissions and health checks

**Runtime user.** The Go services run as UID/GID 10001 (`golive`) with no Linux capabilities
and `no-new-privileges`. Before them, on every `docker compose up`, the one-shot
`init-permissions` container (no network; only the CHOWN, DAC_OVERRIDE and FOWNER
capabilities) prepares what they use and exits:

- the volumes `deploy_avatar_uploads`, `deploy_cover_uploads`, `deploy_post_uploads` and
  `deploy_replay_records` become owned by 10001:10001. SRS still writes recordings as root;
  owning the directory is what lets room-service read and delete them;
- `deploy/secrets` keeps its owner and gets group 10001 with group read access (directories:
  read and traverse), for example `root:10001 0640`. Do not give GID 10001 to a group that
  people log in with on the host: its members could read the JWT keys.

After adding or replacing a file in `deploy/secrets`, run `docker compose up -d` (it re-runs
`init-permissions`), then restart the services that read the file, for example
`docker compose restart user-service api-gateway gift-service room-service im-gateway`.

**Health checks.** Each Go service answers `GET /healthz`, probed every 10s. `docker compose ps`
shows `(healthy)`; `docker inspect --format '{{json .State.Health}}' golive-api-gateway` shows
the last probes. api-gateway waits for user, room, gift and chat-service to be healthy,
im-gateway for Redis, room-service and user-service, and nginx for api-gateway and im-gateway.
Services that migrate the schema at startup get a 5-minute start period.

**Redis** requires the password from `GOLIVE_REDIS_PASSWORD`. Inside the container, `redis-cli`
picks it up automatically (`docker compose exec redis redis-cli ping`).

**Kafka (optional).** Live chat and gift events go through Redis; every service ships with
`kafka.enabled: false`. To use Kafka, set `COMPOSE_PROFILES=kafka` in `deploy/.env` (starts a
single-node KRaft broker, `apache/kafka:3.9.2`) and `kafka.enabled: true` in the
`deploy/configs/*.yaml` of the services that should produce or consume (im-gateway and
chat-service for chat, gift-service for gift events), then deploy.

**pprof** listens on `127.0.0.1` inside each container (ports 6060-6068) and is not reachable
from other containers or the host. To take a profile:

```bash
docker compose exec -T api-gateway wget -qO- 'http://127.0.0.1:6060/debug/pprof/heap' > heap.pprof
```

An empty `service.pprof_addr` in `deploy/configs/<service>.yaml` disables it.

## 6. Access URLs

Web:

```text
http://154.36.185.85
```

OBS publishing server:

```text
rtmp://154.36.185.85/live
```

If `golive.us.ci` resolves to the server and a certificate is installed, nginx automatically uses the HTTPS configuration.

## 7. Common troubleshooting commands

```bash
ssh root@154.36.185.85
cd /srv/golive/app/golive-backend/deploy
docker compose ps -a
docker compose logs init-permissions
docker compose logs --tail=200 nginx
docker compose logs --tail=200 api-gateway
docker compose logs --tail=200 room-service
docker compose logs --tail=200 im-gateway
docker compose logs --tail=200 srs
docker compose exec redis redis-cli ping
```

Rebuild the frontend manually:

```bash
cd /srv/golive/app/golive-web
docker run --rm -v "$PWD:/app" -w /app node:20-alpine sh -lc 'corepack enable && pnpm config set registry https://registry.npmmirror.com && pnpm install --frozen-lockfile && pnpm build'
cd /srv/golive/app/golive-backend/deploy
docker compose up -d --build --remove-orphans
```

Inspect the latest server checkout:

```bash
cd /srv/golive/app
git rev-parse --short HEAD
git log -1 --oneline
```
