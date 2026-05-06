# GoLive Web

`golive-web` 是 GoLive 的生产前端。技术栈为 Vite、React 18、TypeScript、
TanStack Query、Zustand、React Router、Tailwind CSS、Radix UI 和
`mpegts.js`。

## 功能范围

- 首页、搜索、频道页、直播间和 HTTP-FLV 播放。
- 登录、Google 登录、账号封禁页、金币页面。
- 弹幕 WebSocket、礼物、SuperChat、竞猜和近期聊天缓存。
- 创作者工作台：开播准备、预约、动态、管理员、粉丝团、回放和直播控制台。
- 管理后台：仪表盘、创作者审核、内容审核、用户、直播、收入、系统状态。
- 中英日三语文案，主题和语言偏好本地持久化。

## 环境要求

- Node.js >= 20
- pnpm

## 本地启动

```sh
pnpm install
pnpm dev
# http://localhost:5173
```

Vite 开发服务器会把 `/api`、`/ws`、`/live` 代理到本地后端端口：

| 路径 | 目标 |
| ---- | ---- |
| `/api` | `http://localhost:8080` |
| `/ws` | `http://localhost:8081` |
| `/live` | `http://localhost:8082` |

## 环境变量

| 变量 | 说明 | 常用值 |
| ---- | ---- | ------ |
| `VITE_API_BASE` | HTTP API base URL | `/api` |
| `VITE_WS_BASE` | WebSocket base URL | `/ws` |
| `VITE_FLV_BASE` | HTTP-FLV base URL | `/live` |
| `VITE_RTMP_BASE` | OBS 推流地址前缀 | `rtmp://localhost/live` |
| `VITE_GOOGLE_CLIENT_ID` | Google Identity Services client id | 见 `.env.example` |

生产环境默认同源访问：nginx 服务静态文件，并反代 `/api`、`/ws` 和 `/live`。

## 脚本

```sh
pnpm test       # Vitest
pnpm typecheck  # TypeScript project references
pnpm build      # tsc -b && vite build
pnpm lint       # ESLint
pnpm format     # Prettier + Tailwind class sorting
```

## 目录速览

```text
src/
  api/          后端 API 封装
  components/   通用组件和 shadcn/radix 基础组件
  features/     直播间、创作者、账号、媒体等业务组件
  hooks/        通用 hooks
  i18n/         en-US / zh-CN / ja-JP 文案
  lib/          axios、鉴权、CSRF、礼物、等级、缓存等工具
  pages/        路由页面
  stores/       Zustand stores
  styles/       全局样式和直播间样式
```

构建产物、测试报告和本地调试日志由开发环境生成，不纳入版本控制。
