# GoLive Git Bare 部署说明

目标服务器：`171.80.11.198`

## 1. 服务器准备

推荐系统：Ubuntu 22.04 LTS。

开放公网端口：

```text
80    Web 访问
1935  OBS RTMP 推流
443   后续配置 HTTPS 时再开放
```

不要对公网开放 MySQL、Redis、Kafka、etcd、MinIO、Prometheus、Grafana、Jaeger 等端口。

安装基础组件：

```bash
sudo apt update
sudo apt install -y git ca-certificates curl docker.io docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
```

国内服务器建议配置 Docker 镜像加速。不同云厂商镜像地址不同，配置好后执行：

```bash
sudo systemctl restart docker
```

## 2. 创建 bare 仓库

```bash
sudo mkdir -p /srv/git /srv/golive/app
sudo chown -R "$USER":"$USER" /srv/git /srv/golive
git init --bare /srv/git/golive.git
```

安装 hook。第一次服务器还没有 checkout 出 `/srv/golive/app`，所以先从本机把脚本传上去：

```bash
scp scripts/post-receive.golive.example 你的服务器用户名@171.80.11.198:/tmp/post-receive
ssh 你的服务器用户名@171.80.11.198
cp /tmp/post-receive /srv/git/golive.git/hooks/post-receive
chmod +x /srv/git/golive.git/hooks/post-receive
```

后续如果脚本有更新，也可以在服务器上从工作目录复制：

```bash
cp /srv/golive/app/scripts/post-receive.golive.example /srv/git/golive.git/hooks/post-receive
chmod +x /srv/git/golive.git/hooks/post-receive
```

## 3. 本机添加远端并部署

在本机项目根目录执行：

```bash
git remote add prod ssh://你的服务器用户名@171.80.11.198/srv/git/golive.git
git push prod main
```

如果你的本地分支是 `master`：

```bash
git push prod master
```

`post-receive` 会自动完成：

```text
checkout 到 /srv/golive/app
使用 node:20-alpine 构建 golive-web/dist
启动 golive-backend/deploy/docker-compose.yml
```

## 4. OBS 和测试账号

Web 地址：

```text
http://171.80.11.198
```

OBS 推流服务器：

```text
rtmp://171.80.11.198/live
```

demo 用户：

```text
用户名：demo
密码：demo-1718011198
```

## 5. 常用排查命令

```bash
cd /srv/golive/app/golive-backend/deploy
docker compose ps
docker compose logs -f nginx
docker compose logs -f api-gateway
docker compose logs -f room-service
docker compose logs -f srs
```

如果前端没有更新，手动触发一次构建：

```bash
cd /srv/golive/app/golive-web
docker run --rm -v "$PWD:/app" -w /app node:20-alpine sh -lc 'corepack enable && pnpm config set registry https://registry.npmmirror.com && pnpm install --frozen-lockfile && pnpm build'
cd /srv/golive/app/golive-backend/deploy
docker compose up -d --remove-orphans
```
