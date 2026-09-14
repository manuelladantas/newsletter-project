# Daily Digest

A self-hosted daily reading digest. Every morning it pulls the top posts from a handful of tech sources (Dev.to, Medium, ByteByteGo, Hacker News), asks a local LLM (via [Ollama](https://ollama.com)) to pick the ones that match my interests, and shows them in a small web UI where I can favorite the ones worth keeping.

## Why this project exists

This repo is a playground for two things:

1. **Testing my AI-assisted development flow with [Claude Code](https://claude.com/claude-code).** Almost everything here was built by driving Claude Code, with `CLAUDE.md` holding the project conventions it follows.
2. **Trying spec-driven development (SDD).** Each feature goes through a fixed pipeline of custom Claude Code skills (in `.claude/skills/`):
   - `feature-brainstorm` → interview + edge cases → `specs/<feature>/brainstorm.md`
   - `spec-writer` → turns the brainstorm into `specs/<feature>/spec.md`
   - `spec-tdd-writer` → writes failing tests from the spec (red)
   - `spec-tdd-implementer` → writes the code that makes them pass (green)

   The `specs/` folder is the record of that process for every feature shipped so far (`daily-article-digest`, `digest-redesign`, `favorites`).

The end goal is to run this on my personal home server so I have my own daily digest instead of a dozen newsletters in my inbox.

## What it does

- **Scheduler** — at 09:00 (server local time) the backend fetches articles from all sources.
- **Ranking** — the candidate titles are sent to Ollama (`qwen2.5:7b-instruct-q4_K_M`) with an interest profile; the model returns a short list of picks with a one-line reason each.
- **Storage** — the digest is saved to Postgres; the UI always shows the latest one.
- **Favorites** — articles can be favorited from the digest and browsed later in a paginated list.

### API

| Method | Path              | Description                                  |
|--------|-------------------|----------------------------------------------|
| GET    | `/health`         | Pings the DB, returns `ok` or 503            |
| GET    | `/digest/today`   | Latest stored digest (404 if none yet)       |
| POST   | `/digest/run`     | Run the pipeline now and return the digest   |
| GET    | `/favorites`      | Paginated favorites (`?page=N`)              |
| GET    | `/favorites/urls` | All favorited URLs (used to mark the digest) |
| POST   | `/favorites`      | Add a favorite (`{ "url", "title" }`)        |
| DELETE | `/favorites/:id`  | Remove a favorite                            |

## Running it

### Prerequisites

- Docker + Docker Compose
- [Ollama](https://ollama.com) running on the host with the model pulled:
  ```
  ollama pull qwen2.5:7b-instruct-q4_K_M
  ```

### Setup

```
cp .env.example .env   # adjust ports / credentials if needed
```

`OLLAMA_URL` defaults to `http://host.docker.internal:11434`, which reaches the host's Ollama from inside the backend container. On Linux you may need to point it at your host's LAN IP instead.

### Development (hot reload)

```
docker compose up --build
```

Starts Postgres, the Go backend (hot-reloaded with `air`), the Nuxt dev server, and Adminer.

- Frontend: http://localhost:3001
- Backend: http://localhost:8090
- Adminer: http://localhost:8081

To generate a digest immediately instead of waiting for the 09:00 run:

```
curl -X POST http://localhost:8090/digest/run
```

### Production (personal server)

```
docker compose -f docker-compose.yml -f docker-compose.prod.yml up --build -d
```

The override swaps the frontend for a multi-stage build (`frontend/Dockerfile`) that serves the compiled Nuxt app through Nitro and drops the source bind-mount. The backend still runs with `air` for now.

### Running the pieces standalone

Backend (from `backend/`, needs `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `OLLAMA_URL`):

```
go run .
go test ./...
```

Frontend (from `frontend/`):

```
npm install --legacy-peer-deps
npm run dev
```

## Architecture

```
┌──────────┐   NUXT_PUBLIC_API_BASE   ┌───────────────┐   OLLAMA_URL   ┌────────┐
│ frontend │ ───────────────────────▶ │    backend    │ ─────────────▶ │ Ollama │
│  (Nuxt)  │                          │ (Go + Fiber)  │                └────────┘
└──────────┘                          └───────┬───────┘
                                              │
                                      ┌───────▼───────┐      ┌─────────┐
                                      │   Postgres    │ ◀─── │ Adminer │
                                      └───────────────┘      └─────────┘
```

- **`backend/`** — Go HTTP server using Fiber. Organized as vertical slices: `digest/` and `favorites/` each own their handler, business logic, store, and tests. `main.go` only wires the DB, config, scheduler, and routes together.
- **`frontend/`** — Nuxt 4 app (`app/` layout). Page logic lives in composables (`app/composables/`), presentational pieces in `app/components/`, shared types in `shared/types/`. Styled with Tailwind v4.
- **`db`** — Postgres 16, data persisted in the `pgdata` volume.
- **`adminer`** — throwaway web UI for poking at the database.

### Architecture decisions

- **Monorepo.** Backend, frontend, infra, and specs live in one repo so a single Claude Code session (and a single `CLAUDE.md`) has the whole picture.
- **Go + Fiber, vertical slices.** Each feature is a self-contained package instead of controllers/services/repositories layers. It keeps a feature's spec, tests, and code close together, which matches the SDD flow well.
- **Nuxt 4 + Tailwind v4, mobile-first.** No hand-written CSS or `<style>` blocks; base classes target small screens, `md:`/`lg:` variants layer on top. Composables hold state and fetching, components stay presentational.
- **Local LLM via Ollama.** Ranking runs against a model on the host rather than a hosted API, so the digest works offline on the home server with no per-request cost.
- **Docker Compose for everything.** Dev and prod share one `docker-compose.yml`; `docker-compose.prod.yml` is a small override rather than a separate stack.
- **Spec-driven, test-first.** Features are not started without a `spec.md`, and implementation is written to make spec-derived tests pass — the tests are the contract, the spec is the source of truth.

## Repo layout

```
.
├── .claude/skills/        # custom Claude Code skills for the SDD pipeline
├── specs/<feature>/       # brainstorm.md + spec.md per feature
├── backend/               # Go + Fiber API (vertical slices)
├── frontend/              # Nuxt 4 app
├── docker-compose.yml     # dev stack
├── docker-compose.prod.yml# prod override (built frontend)
└── CLAUDE.md              # conventions Claude Code follows in this repo
```
