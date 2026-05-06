# GoLive

GoLive 是一个类 YouTube Live 的直播平台项目，包含 React + TypeScript 前端和
Go 微服务后端。当前仓库已经从早期原型迁移到正式工程结构：

- `golive-web/`：Vite、React、TypeScript 前端，包含直播间、创作者工作台、
  管理后台、消息、金币和账号状态页面。
- `golive-backend/`：Go 后端，按 `app/<service>` 拆分 api-gateway、user、
  room、chat、gift、im-gateway 等服务。
- `golive-backend/deploy/`：本地和服务器使用的 Docker Compose、nginx、SRS、
  观测配置。
- `scripts/`：本地一键启动脚本和服务器 git bare 部署 hook 示例。
- `docs/`：部署、联调和清理说明。

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

## 常用命令

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

当前服务器目标是 `root@154.36.185.85`，推荐通过 git bare 仓库和
`scripts/post-receive.golive.example` 自动部署。详细流程见
`docs/deploy-git-bare.md`。

## 清理提醒

仓库里曾混入浏览器检查目录和测试覆盖率产物。建议清理项、原因和命令见
`docs/cleanup.md`。
