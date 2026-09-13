# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

Early-stage newsletter app skeleton: a Go backend, a Nuxt 4 frontend, and Postgres, wired together with Docker Compose. No tests, no lint config, and no production Dockerfiles exist yet.

## Commands

Primary dev workflow (starts db + backend + frontend + adminer together):
```
docker compose up --build
```
- Backend hot-reloads via `air` on `.go` file changes; frontend runs `nuxt dev`.
- Requires a `.env` file (see `.env.example` for required keys: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`, `BACKEND_PORT`, `FRONTEND_PORT`, `ADMINER_PORT`).

Backend standalone (from `backend/`):
```
go build ./...
go run .
```
Needs `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` set in the environment.

Frontend standalone (from `frontend/`):
```
npm install --legacy-peer-deps
npm run dev
npm run build
npm run preview
```

## Architecture

- **`db`**: `postgres:16-alpine`, data persisted in the `pgdata` volume.
- **`backend`** (`backend/main.go`): single-file Go HTTP server. Opens a `database/sql` connection to Postgres via `lib/pq`. Currently exposes only `GET /health`, which pings the DB and returns 200/503 — no router/framework and no other endpoints yet.
- **`frontend`** (`frontend/`): Nuxt 4 app using the `app/` source layout (`app/app.vue`, `app/components/`, `app/composables/`, `app/assets/css/main.css`) with shared types in `shared/types/` — all auto-imported. Styled with Tailwind v4 via the `@tailwindcss/vite` plugin (no `tailwind.config`; the CSS entry is `app/assets/css/main.css`). Reads the backend URL from `runtimeConfig.public.apiBase`, set via the `NUXT_PUBLIC_API_BASE` env var (defaults to `http://localhost:8090`). `app/app.vue` renders the daily digest via the `useDigest` composable and the `Digest*` components.
- **`adminer`**: web UI for inspecting the Postgres DB, exposed on `ADMINER_PORT`.

Env vars flow from the root `.env` file through `docker-compose.yml`: `POSTGRES_*` vars configure the `db` service and are remapped to `DB_*` for `backend`; `NUXT_PUBLIC_API_BASE` is passed to `frontend`.

Both `Dockerfile.dev` files (backend and frontend) are dev-only — they bind-mount source for hot reload and are not suited for production.

## Architecture decisions

- **Monorepo**: backend, frontend, and infra (Docker Compose) live together in a single repository.
- **Backend**: Go using the **Fiber** web framework, organized as **vertical slices** — each feature/use case owns its own handler, business logic, and data access, rather than being split across horizontal layers (controllers/services/repositories). The current `backend/main.go` is a pre-Fiber skeleton and will be migrated to this structure.
- **Frontend**: Vue 3 via **Nuxt 4**, styled with **Tailwind v4** utility classes (no hand-written CSS or `<style>` blocks). Components should be written mobile-first (base classes target small screens, larger breakpoints layered on top via `md:`/`lg:` variants). Page logic lives in composables (`app/composables/`), presentational pieces in `app/components/`.
