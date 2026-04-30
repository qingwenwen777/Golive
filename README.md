# GoLive

A YouTube-style live streaming front-end (React + TypeScript) built to connect
to a Go microservice backend (api-gateway / im-gateway / SRS). The repository
currently contains two parts:

- **Prototype** (`GoLive.html`, `styles.css`, `components/*.jsx`, `tweaks-panel.jsx`) —
  a single-file Babel-standalone React prototype used as the visual reference.
  Kept in-place during migration; will be cleaned up at the final stage.
- **Production app** (`golive-web/`) — the Vite + TypeScript project being
  migrated towards.

## Local development

```sh
cd golive-web
pnpm install
pnpm dev
# http://localhost:5173
```

Node.js >= 20 and pnpm are required.

## Docker

```sh
cd golive-web
cp .env.example .env
docker compose up --build
# http://localhost
```

The container is a multi-stage build (node:20-alpine → nginx:alpine) and serves
the SPA with history-mode fallback.
