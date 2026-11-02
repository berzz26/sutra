# searcher

Self-hostable semantic video search: submit a YouTube URL or upload a
transcript, get back the moments that answer your natural-language query.

**Status: Phase 0 — repository foundation.** The backend runs, connects to
PostgreSQL, migrates its schema, serves health endpoints and shuts down
gracefully. The frontend is an app shell wired to a health endpoint.
Ingestion, embeddings, search, and reranking are later phases — see
`docs/architecture.md` and `docs/knowledge.md`.

## Stack

- **Backend:** Go, Fiber, pgx (PostgreSQL)
- **Frontend:** React, TypeScript, Vite, React Router
- **Database:** PostgreSQL 16
- **Inference (later):** ONNX Runtime, BGE-small-en-v1.5 embeddings,
  mxbai-rerank-xsmall-v1 reranker — all local, no external AI APIs
- **Artifact store (optional):** any S3-compatible endpoint (e.g. MinIO)
- **Deployment:** Docker Compose (Postgres + backend + frontend)

## Repository structure

```
searcher/
├── backend/
│   ├── cmd/server/             entry point + embedded migrations
│   │   ├── main.go
│   │   └── migrations/0001_init.sql
│   ├── internal/
│   │   ├── api/                routes + handlers (health, /api/v1)
│   │   ├── config/             environment configuration
│   │   ├── domain/             plain business structs (video, cue, chunk, job)
│   │   ├── persistence/        connection pool + migration runner
│   │   ├── server/             Fiber app assembly, graceful shutdown
│   │   ├── ingestion/          transcript ingestion (scaffold, later phase)
│   │   ├── processing/         cues → search chunks (scaffold, later phase)
│   │   ├── embedding/          BGE-small-en-v1.5 via ONNX (scaffold, later)
│   │   ├── retrieval/          vector + text search (scaffold, later)
│   │   ├── rerank/             reranking (scaffold, later)
│   │   └── jobs/               background job execution (scaffold, later)
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── api/                API client abstraction + typed errors
│   │   ├── components/         layout shell, error boundary
│   │   ├── pages/              route pages (search placeholder, status)
│   │   ├── App.tsx             router
│   │   └── main.tsx            bootstrap
│   ├── .env.example
│   ├── Dockerfile
│   ├── package.json
│   ├── tsconfig*.json
│   └── vite.config.ts          dev + preview proxy to backend
├── docker-compose.yml          postgres + backend + frontend
├── docs/
│   ├── architecture.md         design decisions and rationale
│   ├── rules.md                engineering rules for contributors
│   └── knowledge.md            implementation/test log (keep updated)
├── .env.example
└── Makefile
```

## Local development

### Prerequisites

- Go 1.22+
- Node 22+
- PostgreSQL (local or remote; schema is applied automatically)

### Backend

```sh
cp .env.example .env
# edit DATABASE_URL to point at your PostgreSQL instance
make run-backend
```

Environment variables (all options documented in `.env.example` and
`docs/architecture.md`):

| Variable | Required | Purpose |
|---|---|---|
| `DATABASE_URL` | yes | PostgreSQL connection string |
| `SERVER_HOST` | no | Listen host (default `0.0.0.0`) |
| `SERVER_PORT` | no | Listen port (default `8080`) |
| `LOG_LEVEL` | no | `debug`/`info`/`warn`/`error` |
| `ARTIFACT_STORE_ENABLED` | no | Enables MinIO/S3 settings |
| `MINIO_*` | if enabled | Endpoint, keys, bucket, region, SSL flag |

Verify:

```sh
curl http://localhost:8080/healthz   # {"status":"ok"}
curl http://localhost:8080/readyz    # {"status":"ready"} (503 if DB down)
```

### Frontend

```sh
cd frontend
cp .env.example .env         # set VITE_API_PROXY_TARGET to the backend URL
npm install
npm run dev                  # http://localhost:5173, proxies /api to backend
```

The status page (`http://localhost:5173/status`) proves the frontend ↔
backend path works; it calls `/api/v1/health` and shows the result.

### One-command stack (Docker Compose)

```sh
docker compose up --build -d
```

- Frontend: http://localhost:4173 (proxies `/api` to the backend)
- Backend: http://localhost:8080
- Postgres: localhost:5432 (`searcher` / `searcher`), data in a named volume

## Graceful shutdown

`SIGINT`/`SIGTERM` stop the listener, drain in-flight requests (15s budget),
then exit 0. The database pool is closed after the server drains.

## Documentation

- `docs/architecture.md` — system shape, layering, and the reasoning behind
  each decision (cues vs chunks, table-backed jobs, embedded migrations…).
- `docs/rules.md` — engineering rules every change must follow (including:
  no excessive commenting; every implemented feature gets logged in
  `docs/knowledge.md`).
- `docs/knowledge.md` — running log of what is implemented and tested.
