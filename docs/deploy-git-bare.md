# GoLive Git Bare 部署说明

目标服务器：`root@154.36.185.85`

本项目的服务器部署方式是：本机 push 到服务器 bare 仓库，服务器
`post-receive` hook 自动 checkout、构建前端并重启 Compose 服务。

## 1. 服务器准备

推荐系统：Ubuntu 22.04 LTS。

公网只开放必要端口：

```text
80    Web HTTP
443   Web HTTPS
1935  OBS RTMP 推流
```

不要对公网开放 MySQL、Redis、Kafka、etcd、MinIO、Prometheus、Grafana、
Jaeger 等内部端口。

安装基础组件：

```bash
apt update
apt install -y git ca-certificates curl docker.io docker-compose-plugin
systemctl enable --now docker
```

如果在国内网络环境，建议配置 Docker 镜像源，配置后执行：

```bash
systemctl restart docker
```

## 2. 创建 bare 仓库

```bash
mkdir -p /srv/git /srv/golive/app
git init --bare /srv/git/golive.git
```

安装部署 hook：

```bash
scp scripts/post-receive.golive.example root@154.36.185.85:/tmp/post-receive
ssh root@154.36.185.85
cp /tmp/post-receive /srv/git/golive.git/hooks/post-receive
chmod +x /srv/git/golive.git/hooks/post-receive
```

如果 hook 脚本后续有更新，可以在服务器工作目录存在后执行：

```bash
cp /srv/golive/app/scripts/post-receive.golive.example /srv/git/golive.git/hooks/post-receive
chmod +x /srv/git/golive.git/hooks/post-receive
```

## 3. 本机添加远端并部署

当前本地分支是 `master`，推荐远端名为 `prod`：

```bash
git remote add prod ssh://root@154.36.185.85/srv/git/golive.git
git push prod master
```

如果远端已经存在：

```bash
git remote set-url prod ssh://root@154.36.185.85/srv/git/golive.git
git push prod master
```

`post-receive` 会自动完成：

```text
checkout 到 /srv/golive/app
使用 node:20-alpine 安装依赖并构建 golive-web/dist
重新构建并启动 golive-backend/deploy/docker-compose.yml 中的服务镜像
对 nginx / srs 等单文件挂载配置做漂移检测，必要时强制重建容器
输出 docker compose ps
```

> 边缘容器（nginx、srs）通过单文件 bind-mount 挂载配置（如
> `nginx.https.conf`、`srs.conf`）。Docker 在创建容器时按 inode 绑定挂载，而
> `git checkout -f` 会原子替换文件（生成新 inode），导致长期运行的容器仍读旧
> 文件；`docker compose up -d` 不会因单文件内容变化而重建，`nginx -s reload`
> 也无法解决（挂载的还是旧 inode）。因此 hook 在部署末尾比对宿主机配置与容器
> 内实际挂载文件的内容哈希，仅在发生漂移时对相应服务执行
> `docker compose up -d --force-recreate --no-deps <service>`，从根本上避免配置
> 不生效的问题。注意：`srs.conf` 变更触发的 srs 重建会短暂中断直播推流。

部署 hook 默认接受 `main` 和 `master`。服务器变量可覆盖：

```bash
DEPLOY_BRANCH=main
FALLBACK_BRANCH=master
WORK_TREE=/srv/golive/app
GIT_DIR=/srv/git/golive.git
```

## 4. 访问地址

Web：

```text
http://154.36.185.85
```

OBS 推流服务器：

```text
rtmp://154.36.185.85/live
```

如果域名 `golive.us.ci` 已解析并安装证书，nginx 会自动使用 HTTPS 配置。

## 5. 常用排查命令

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

手动重新构建前端：

```bash
cd /srv/golive/app/golive-web
docker run --rm -v "$PWD:/app" -w /app node:20-alpine sh -lc 'corepack enable && pnpm config set registry https://registry.npmmirror.com && pnpm install --frozen-lockfile && pnpm build'
cd /srv/golive/app/golive-backend/deploy
docker compose up -d --build --remove-orphans
```

查看最近一次服务器 checkout：

```bash
cd /srv/golive/app
git rev-parse --short HEAD
git log -1 --oneline
```
