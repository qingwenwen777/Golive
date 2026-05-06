# GoLive

GoLive 是一个类 YouTube Live 的直播平台项目，包含 React + TypeScript 前端、
Go 微服务后端、直播流服务和本地/生产部署配置。

## 项目结构

| Path | Description |
| ---- | ----------- |
| `golive-web/` | Vite + React 前端，包含直播间、创作者工作台、管理后台、消息、金币和账号状态页面 |
| `golive-backend/` | Go 后端，按 `app/<service>` 拆分 api-gateway、user、room、chat、gift、im-gateway 等服务 |
| `golive-backend/deploy/` | Docker Compose、nginx、SRS、TLS 和观测配置 |
| `scripts/` | 本地一键启动脚本和 git bare 部署 hook 示例 |
| `docs/` | 部署和前后端联调文档 |

## 环境要求

- Node.js >= 20
- pnpm
- Go >= 1.22
- Docker Engine + Docker Compose plugin

## 本地开发

前端单独开发：

```sh
cd golive-web
pnpm install
pnpm dev
# http://localhost:5173
```

完整联调：

```sh
# Windows PowerShell
powershell -File scripts/dev.ps1

# Linux/macOS/WSL/Git Bash
bash scripts/dev.sh
```

脚本会先构建 `golive-web/dist`，再启动 `golive-backend/deploy/docker-compose.yml`
中的 nginx、SRS、MySQL、Redis、Kafka、Go 服务和观测组件。

## 验证

```sh
cd golive-web
pnpm test
pnpm typecheck
pnpm build
```

```sh
cd golive-backend
go test ./...
```

## 部署

生产部署使用服务器上的 git bare 仓库和 `scripts/post-receive.golive.example`
自动 checkout、构建前端并重启 Docker Compose 服务。详细流程见
`docs/deploy-git-bare.md`，联调说明见 `docs/integration.md`。
