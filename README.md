# GoLive

GoLive is a YouTube Live-style streaming platform with a React + TypeScript frontend,
Go microservices, streaming services, and local and production deployment configurations.

## Project structure

| Path | Description |
| ---- | ----------- |
| `golive-web/` | Vite + React frontend with live rooms, creator studio, admin console, messages, coins, and account status pages |
| `golive-backend/` | Go backend split into api-gateway, user, room, chat, gift, im-gateway, and other services under `app/<service>` |
| `golive-backend/deploy/` | Docker Compose, nginx, SRS, TLS, and observability configuration |
| `scripts/` | One-command local startup scripts and an example Git bare deployment hook |
| `docs/` | Deployment and frontend/backend integration documentation |

## Prerequisites

- Node.js >= 20
- pnpm
- Go >= 1.25
- Docker Engine + Docker Compose plugin

## Local development

Run the frontend on its own:

```sh
cd golive-web
pnpm install
pnpm dev
# http://localhost:5173
```

Run the complete integration stack:

```sh
# Windows PowerShell
powershell -File scripts/dev.ps1

# Linux/macOS/WSL/Git Bash
bash scripts/dev.sh
```

The scripts first build `golive-web/dist`, then start nginx, SRS, MySQL, Redis, Kafka,
Go services, and observability components from `golive-backend/deploy/docker-compose.yml`.

## Validation

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

## Deployment

Production deployment uses a bare Git repository on the server and `scripts/post-receive.golive.example`
to check out the code, build the frontend, and restart Docker Compose services automatically. See
`docs/deploy-git-bare.md` for deployment instructions and `docs/integration.md` for integration notes.
