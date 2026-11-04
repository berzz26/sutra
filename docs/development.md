# Development

This guide covers running Sutra from source, the test/lint loop, and the
repository layout. For the container-first path, see
[Installation](installation.md).

## Toolchain

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.26+ | API and command-line tools. `api/go.mod` declares `go 1.26.0`; recent Go toolchains can auto-download it if needed. |
| Node.js | 22.12+ | Frontend build and dev server. |
| npm | Bundled with Node 22 | Uses `package-lock.json` via `npm ci`. |
| Docker + Compose v2.20+ | Current stable | PostgreSQL/pgvector for development. |
| yt-dlp + Deno | Recent releases | Only needed for YouTube ingestion when running on the host. Uploads and search do not need them. |

The Docker images pin and verify yt-dlp, Deno, ONNX Runtime, and the model.
Host development uses whatever versions you install locally.

## First-time setup

From the repository root:

```sh
make dev-deps
```

This target:

- downloads the pinned BGE-small-en-v1.5 ONNX model into
  `api/embedding/models/bge-small-en-v1.5/` if missing;
- downloads the pinned ONNX Runtime into
  `api/third_party/onnxruntime/onnxruntime-linux-x64-1.31.1/` if missing;
- runs `npm ci` in `frontend/` if `node_modules` is missing;
- runs `go mod download` in `api/`.

Both downloaded directories are ignored by Git. Do not commit model weights or
native runtime archives.

## Run everything on the host

```sh
make dev
```

This target:

1. starts PostgreSQL with pgvector through
   `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d db`;
2. publishes PostgreSQL on `127.0.0.1:5432`;
3. runs the Go API with `go run ./cmd/server` on
   <http://localhost:8080>;
4. runs the Vite dev server on <http://localhost:5173>;
5. stops the API and Vite processes when you press Ctrl-C. The PostgreSQL
   container keeps running.

The Vite dev server proxies `/api` to `http://localhost:8080` by default. Copy
`frontend/.env.example` to `frontend/.env` to change
`VITE_API_PROXY_TARGET`.

If you prefer separate terminals:

```sh
# Terminal 1: database
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d db

# Terminal 2: API
cd api
DATABASE_URL='postgresql://searcher:searcher@127.0.0.1:5432/searcher' \
  go run ./cmd/server

# Terminal 3: frontend
cd frontend
npm run dev
```

The API applies migrations automatically at startup. The default host
`DATABASE_URL` is only a development convention; set your own value if your
database differs.

## Tests and checks

```sh
make test          # Go tests + frontend typecheck
make lint          # go vet + frontend typecheck + Compose validation
make fmt           # gofmt -w .
```

Equivalent commands:

```sh
cd api
gofmt -w .
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go build ./...

cd ../frontend
npm ci
npm run typecheck
npm run build
```

There is currently no frontend test runner; the frontend checks are TypeScript
typechecking and a production build. Backend tests are unit tests and do not
require a live PostgreSQL instance unless a package is changed to add an
integration test. The end-to-end HTTP smoke test does require the running
stack:

```sh
make smoke
BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh  # bypass nginx
```

## Docker-only development

Build and run the production-shaped stack:

```sh
docker compose up --build -d
docker compose logs -f api frontend
```

Rebuild only one service:

```sh
docker compose build api
docker compose up -d api frontend
```

Validate Compose files without starting containers:

```sh
make config
# expands to:
# docker compose config --quiet
# docker compose --profile artifacts config --quiet
```

## Repository layout

```text
.
├── api/                          Go module (module path kept as
│   │                             github.com/berzz/searcher/backend)
│   ├── cmd/
│   │   ├── server/               API entry point + embedded migrations
│   │   ├── backfill/             enqueue existing transcripts for indexing
│   │   ├── evaluate-search/      retrieval evaluation helper
│   │   ├── onnx-smoke/           ONNX runtime/model smoke helper
│   │   └── tokenizer-smoke/      tokenizer smoke helper
│   ├── internal/
│   │   ├── api/                  routes and handlers
│   │   ├── artifact/            optional S3-compatible storage
│   │   ├── config/              environment loading and validation
│   │   ├── domain/              business structs
│   │   ├── embedding/           ONNX session, tokenizer, pooling
│   │   ├── ingestion/           URL validation, yt-dlp, uploads
│   │   ├── jobs/                in-process PostgreSQL job workers
│   │   ├── persistence/         pgx pool, migrations, queries
│   │   ├── processing/          cue chunking and embedding orchestration
│   │   ├── rerank/              reserved; not implemented
│   │   ├── retrieval/           vector + lexical candidates, RRF
│   │   ├── server/              Fiber assembly and graceful shutdown
│   │   └── transcript/          VTT/SRT parsing
│   └── Dockerfile
├── deploy/model-init/             one-shot checksum-verified model downloader
├── docs/                          operator and contributor documentation
├── frontend/                      React + TypeScript + Vite SPA
│   ├── src/api/                   typed API client and endpoints
│   ├── src/components/            layout, UI primitives, error boundary
│   ├── src/pages/                 search/status/not-found pages
│   ├── nginx.conf                 production SPA + /api proxy
│   └── Dockerfile
├── scripts/                       model/ORT fetch helpers, smoke test, fixture
├── .env.example                   complete Compose configuration reference
├── docker-compose.yml             default single-command stack
├── docker-compose.dev.yml         loopback-only dev port overrides
├── Makefile                       developer entry points
└── THIRD_PARTY_NOTICES.md         dependency licenses and pinned artifacts
```

The Go module path `github.com/berzz/searcher/backend` is retained for import
compatibility even though the source directory is `api/`. Do not rename it
without a coordinated migration.

## Engineering conventions

Read [docs/rules.md](rules.md) for the full list. In short:

- Keep the single-binary, single-database architecture. No Redis/Kafka/RabbitMQ
  and no external AI APIs.
- Keep raw cues (`transcript_cues`) separate from derived chunks
  (`transcript_chunks`).
- Store background work in PostgreSQL and process it with in-process workers.
- Avoid speculative abstractions and placeholder logic.
- Use `gofmt`, `go vet`, TypeScript strict mode, and the existing Fiber/pgx
  stack.
- Add tests for behavior changes and update documentation when configuration,
  APIs, or operations change.
- Never commit `.env`, secrets, model weights, native runtime archives, or
  generated build output.

See [CONTRIBUTING.md](../CONTRIBUTING.md) for the pull-request process.

## Manual indexing and evaluation

Re-enqueue transcripts that have cues but no chunks:

```sh
cd api
go run ./cmd/backfill
```

Run the retrieval evaluation helper against the configured database:

```sh
cd api
go run ./cmd/evaluate-search
```

The labels in `api/cmd/evaluate-search/queries.json` are tied to a development
dataset. Replace them with labels for your own content before treating the
metrics as meaningful.

## Debugging

Follow logs:

```sh
docker compose logs -f api model-init db frontend
```

Open a database shell:

```sh
make db-shell
# or with the dev override:
docker compose -f docker-compose.yml -f docker-compose.dev.yml exec db \
  psql -U searcher -d searcher
```

Inspect migration state:

```sql
SELECT version, applied_at FROM schema_migrations ORDER BY version;
```

Check processing state and jobs:

```sql
SELECT video_id, status, stage, progress_percent, chunk_count, last_error
FROM video_processing ORDER BY updated_at DESC;

SELECT id, video_id, kind, status, attempts, last_error, run_after
FROM jobs ORDER BY created_at DESC LIMIT 20;
```

More failure modes are covered in [Troubleshooting](troubleshooting.md).
