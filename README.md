# searcher

Self-hostable semantic video search: submit a YouTube URL or upload a
transcript, get back the moments that answer your natural-language query.

**Status: Phase 2 — transcript indexing.** The backend ingests YouTube videos
that already expose manual or automatic subtitles and accepts VTT/SRT uploads,
then asynchronously chunks and embeds transcripts locally. Semantic retrieval
and reranking are later phases — see
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
│   │   ├── ingestion/          YouTube and VTT/SRT transcript ingestion
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
| `YTDLP_BINARY` | no | yt-dlp executable (default `yt-dlp`) |
| `YTDLP_TIMEOUT_SECONDS` | no | Per-command timeout (default 120) |
| `TRANSCRIPT_MAX_UPLOAD_MB` | no | Upload limit (default 20 MB) |
| `TRANSCRIPT_MAX_CONCURRENT_INGESTIONS` | no | Concurrent YouTube extractions (default 2) |
| `EMBEDDING_*` / `ONNXRUNTIME_SHARED_LIBRARY_PATH` | no | Local BGE-small ONNX model, tokenizer, batching, and timeout settings |
| `PROCESSING_WORKERS` | no | In-process PostgreSQL queue worker concurrency (default 2) |
| `SHUTDOWN_TIMEOUT_SECONDS` | no | Graceful HTTP/worker shutdown budget (default 15 seconds) |
| `SEARCH_CANDIDATE_POOL` | no | Candidates retrieved independently per search method (default 100) |
| `SEARCH_RRF_K` | no | Reciprocal Rank Fusion smoothing constant (default 60) |
| `SEARCH_TIMEOUT` | no | Query embedding and database search timeout (default 10s) |

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

## Transcript ingestion

`POST /api/v1/videos` accepts `{"url":"https://youtu.be/VIDEO_ID", "language":"en"}`.
The backend validates the URL, asks `yt-dlp` for metadata, prefers a matching
manual subtitle track, then falls back to matching automatic captions. Without
a requested language it prefers English and otherwise selects a deterministic
available language. It downloads only the selected subtitle file; a video
without subtitles returns `NO_TRANSCRIPT`.

Subtitle selection preserves yt-dlp's exact language key. For an explicit
language request, manual exact matches win first, followed by manual variants;
automatic original-language keys such as `en-orig`/`hi-orig` then win over the
regular generic key. Exact regional requests such as `en-US` remain more
specific than generic `en-orig`, and remaining regional matches are selected
deterministically. An unavailable requested language returns
`LANGUAGE_UNAVAILABLE`.

`GET /api/v1/videos/subtitles?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DVIDEO_ID`
lists compatible manual subtitle tracks only, with their exact yt-dlp language
keys, source, and formats. Automatic captions are not shown by this route.
Submit the selected `language` and
`source` back to `POST /api/v1/videos` as `subtitle_key` and
`subtitle_source` to fetch that exact track.

`POST /api/v1/transcripts` accepts multipart fields `file`, `title`,
`original_url`, and `language`. The file must be valid VTT or SRT; without a
title its filename is used. `GET /api/v1/videos`, `GET /api/v1/videos/:id`,
`GET /api/v1/videos/:id/transcript`, and `DELETE /api/v1/videos/:id` provide
the library, paginated raw cues, and cleanup operations. MinIO is optional:
when enabled the original subtitle is stored under a transcript object key;
when disabled structured cues still persist and the artifact key is null.

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

## Transcript backfill and processing

Newly completed ingestions enqueue an `embed` job in PostgreSQL and return
without waiting for ONNX inference. In-process workers claim jobs with
`FOR UPDATE SKIP LOCKED`, retry transient failures up to five attempts with
backoff, and upsert chunks by `(video_id, seq)` so retries are safe.

To enqueue existing transcripts that have cues but no chunks, run this from
the `api` directory:

```sh
go run ./cmd/backfill
```

The command is repeatable and can be resumed after interruption. Startup
requires pgvector; migration failure is reported clearly rather than falling
back to text or JSON storage.

## Search

`POST /api/v1/search` accepts JSON such as:

```json
{"query":"How does database replication work?","limit":10}
```

Use `video_id` to restrict results to one video. Search embeds the query with
the resident BGE model, retrieves vector candidates using cosine distance and
lexical candidates using PostgreSQL English full-text search, then combines
their ranks with RRF. The returned `score` is the RRF score, not a probability;
`semantic_score` is cosine similarity (`1 - distance`) and `lexical_score` is
PostgreSQL `ts_rank_cd` when that method returned the chunk.

## Documentation

- `docs/architecture.md` — system shape, layering, and the reasoning behind
  each decision (cues vs chunks, table-backed jobs, embedded migrations…).
- `docs/rules.md` — engineering rules every change must follow (including:
  no excessive commenting; every implemented feature gets logged in
  `docs/knowledge.md`).
- `docs/knowledge.md` — running log of what is implemented and tested.
