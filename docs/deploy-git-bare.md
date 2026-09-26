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

Do not expose internal ports for MySQL, Redis, Kafka, etcd, MinIO, Prometheus, Grafana,
Jaeger, or other internal services to the public internet.

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

## 3. Add the remote locally and deploy

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
Rebuild and start service images in golive-backend/deploy/docker-compose.yml
Check single-file configuration mounts for drift in nginx / srs and force-recreate containers when needed
Print docker compose ps
```

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
```

## 4. Access URLs

Web:

```text
http://154.36.185.85
```

OBS publishing server:

```text
rtmp://154.36.185.85/live
```

If `golive.us.ci` resolves to the server and a certificate is installed, nginx automatically uses the HTTPS configuration.

## 5. Common troubleshooting commands

```bash
ssh root@154.36.185.85
cd /srv/golive/app/golive-backend/deploy
docker compose ps
docker compose logs --tail=200 nginx
docker compose logs --tail=200 api-gateway
docker compose logs --tail=200 room-service
docker compose logs --tail=200 im-gateway
docker compose logs --tail=200 srs
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
