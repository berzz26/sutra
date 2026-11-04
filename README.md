# Sutra

**Self-hosted semantic search for video transcripts.** Bring in a YouTube video
or a VTT/SRT transcript, then find the exact moments that answer a natural-language
question. Everything runs on your machine: PostgreSQL + pgvector for storage,
ONNX Runtime for local embeddings, and no external AI APIs.

[![CI](https://github.com/berzz26/searcher/actions/workflows/ci.yml/badge.svg)](https://github.com/berzz26/searcher/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> **Project status:** early but functional. Ingestion, background indexing,
> hybrid search, the web UI, and the Docker Compose stack are implemented.
> There is no authentication or multi-user permission model yet; do not expose a
> shared instance directly to the internet. See [Current limitations](#current-limitations).

## Why Sutra

- **Search by meaning, not just keywords.** Transcript chunks are embedded with
  BGE-small-en-v1.5 and combined with PostgreSQL full-text search through
  Reciprocal Rank Fusion.
- **Private by default.** Transcripts, embeddings, and search indexes stay in
  your PostgreSQL volume. The only outbound request for ingestion is the
  YouTube subtitle fetch performed by `yt-dlp`.
- **Small operational footprint.** One Go API that also runs its workers, one
  static frontend, one PostgreSQL database. No Redis, Kafka, RabbitMQ, or
  separate worker service.
- **Reproducible first run.** Pinned and checksum-verified model, ONNX Runtime,
  yt-dlp, and Deno artifacts. The model is downloaded once into a named volume.
- **Useful output.** Results include the matched transcript text, video title,
  start/end timestamps, and a YouTube link at the exact moment.

## Quick start

### Requirements

- Linux **amd64** (x86_64). ARM64 is not supported end to end yet.
- Docker Engine with Compose v2.20+.
- 4 GB RAM and about 4 GB of free disk space recommended.
- Internet access on the first run to pull images and download the embedding
  model. Later starts reuse the named volumes.

### Start the stack

From a clean checkout:

```sh
docker compose up --build -d
```

Wait for the stack to become healthy:

```sh
docker compose ps
```

Then open:

| Service | URL |
|---|---|
| Web UI | <http://localhost:3000> |
| API (loopback only) | <http://localhost:8080> |
| API through the UI proxy | <http://localhost:3000/api/v1/health> |

First startup builds the images, downloads the pinned BGE-small-en-v1.5 model
(about 127 MiB), and runs the database migrations. Follow progress with:

```sh
docker compose logs -f model-init api
```

No `.env` file is required. To change ports, credentials, or performance
settings, copy `.env.example` to `.env` first. See
[Configuration](docs/configuration.md) for every option.

### Verify it works

```sh
curl -fsS http://localhost:3000/healthz
curl -fsS http://localhost:3000/api/v1/health
curl -fsS http://localhost:8080/readyz
```

Run the real end-to-end smoke test (upload, index, hybrid search):

```sh
make smoke
# or
./scripts/smoke-test.sh
```

### Stop

```sh
docker compose down       # keeps the database and model volumes
docker compose down -v    # deletes all persisted data and the model cache
```

## How it works

```text
Browser ──> nginx (React SPA, /api proxy) ──> Go API (Fiber)
                                                │
                            ┌───────────────────┼────────────────────┐
                            │                   │                    │
                     PostgreSQL 16          yt-dlp + Deno       ONNX Runtime
                     pgvector + FTS         (subtitles only)    BGE-small-en-v1.5
                            │
                      in-process workers
```

1. **Ingest.** Submit a YouTube URL or upload a `.vtt`/`.srt` file. YouTube
   ingestion downloads only the selected subtitle track, never audio or video.
2. **Store.** Raw cues are persisted transactionally in PostgreSQL. Identical
   uploads deduplicate by SHA-256; YouTube videos deduplicate by video ID.
3. **Chunk and embed.** A PostgreSQL-backed job queue is consumed by workers in
   the same Go process. Cues are grouped into overlapping word-bounded chunks
   and embedded locally.
4. **Search.** The query is embedded and also converted to a PostgreSQL
   full-text query. Vector and lexical candidates are merged with Reciprocal
   Rank Fusion, returning timestamped transcript moments.

Read [Architecture](docs/architecture.md) for the design rationale and
[docs/](docs/) for the full operator and API documentation.

## Common API calls

All examples go through the frontend's same-origin proxy. Use
`http://localhost:8080/api/v1` instead if you want to call the API directly.

Ingest a YouTube video using its available English subtitles:

```sh
curl -X POST http://localhost:3000/api/v1/videos \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://www.youtube.com/watch?v=VIDEO_ID","language":"en"}'
```

Discover the available subtitle tracks first:

```sh
curl 'http://localhost:3000/api/v1/videos/subtitles?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DVIDEO_ID'
```

Upload a transcript:

```sh
curl -X POST http://localhost:3000/api/v1/transcripts \
  -F 'file=@talk.vtt;type=text/vtt' \
  -F 'title=Example talk' \
  -F 'language=en'
```

Search:

```sh
curl -X POST http://localhost:3000/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"How does database replication work?","limit":10}'
```

The `score` field is an RRF fusion score, not a probability. See
[Search](docs/search.md) and the [API reference](docs/api.md) for details and
error handling.

## Configuration

The stack works out of the box for local use. For anything reachable by other
people:

1. Set a strong `POSTGRES_PASSWORD` (and MinIO credentials if enabled).
2. Put an authenticating TLS reverse proxy in front of the frontend; Sutra has
   no built-in login.
3. Keep the API and database ports bound to loopback, as the default Compose
   file does.

Start with [.env.example](.env.example) and
[Configuration](docs/configuration.md). The default database password is a
local development default, not a secret.

## Development

Host toolchains:

- Go 1.26+
- Node.js 22.12+ (required by Vite 7)
- Docker with Compose v2 (for PostgreSQL/pgvector)

```sh
make dev-deps   # model, ONNX Runtime, npm and Go dependencies
make dev        # PostgreSQL in Docker; API and Vite dev server on the host
make test       # Go tests + frontend typecheck
make lint       # go vet, TypeScript typecheck, Compose validation
```

The dev server runs at <http://localhost:5173> and proxies `/api` to
<http://localhost:8080>. See [Development](docs/development.md) and
[CONTRIBUTING.md](CONTRIBUTING.md).

## Documentation

| Document | Contents |
|---|---|
| [Installation](docs/installation.md) | Clean install, first-run checks, upgrades, offline model setup |
| [Configuration](docs/configuration.md) | Every environment variable and port |
| [Development](docs/development.md) | Host toolchain, `make dev`, tests, repository layout |
| [Architecture](docs/architecture.md) | Components, request flows, design trade-offs |
| [Database](docs/database.md) | Schema, migrations, indexes, lifecycle, job queue |
| [Ingestion](docs/ingestion.md) | YouTube and upload behavior, subtitle selection, errors |
| [Search](docs/search.md) | Embeddings, lexical search, RRF, result semantics |
| [Models](docs/models.md) | Pinned model, checksums, verification, replacement |
| [API reference](docs/api.md) | Endpoints, payloads, status codes |
| [Backup and restore](docs/backup-and-restore.md) | Volumes, `pg_dump`, recovery |
| [Troubleshooting](docs/troubleshooting.md) | Common startup, model, and indexing failures |

Community and project files:

- [CONTRIBUTING.md](CONTRIBUTING.md)
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- [SECURITY.md](SECURITY.md)
- [SUPPORT.md](SUPPORT.md)
- [CHANGELOG.md](CHANGELOG.md)
- [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)

## Current limitations

- **No authentication.** Anyone who can reach port 3000 can read, ingest, and
  delete data. Use a reverse proxy with authentication for shared deployments.
- **Linux amd64 only.** The pinned ONNX Runtime, yt-dlp, and Deno binaries are
  x86_64 builds. Apple Silicon, ARM servers, and Windows containers are not
  supported by the provided images.
- **YouTube extraction is best effort.** It depends on YouTube's current
  behavior and on `yt-dlp`. Rate limits, consent walls, region blocks, and
  player changes can cause extraction failures. Cookies/proxies are not exposed
  as configuration.
- **English-centric lexical search and model.** The embedding model is English;
  PostgreSQL full-text search uses the `english` configuration.
- **No reranking, answer generation, or summarization.** Search returns ranked
  transcript chunks, not synthesized answers.
- **Single-node workers.** Jobs are coordinated through PostgreSQL and intended
  for one API container. There is no distributed worker coordination.
- **No GPU acceleration.** Inference uses the CPU ONNX Runtime build.
- **The bundled MinIO profile is optional and unverified.** Artifact storage is
  not required; point `MINIO_*` at any S3-compatible store if you need original
  subtitle files retained.

## Security and privacy

Transcripts and embeddings are stored in PostgreSQL. The embedding model runs
locally. YouTube ingestion contacts YouTube through `yt-dlp`; no transcript data
is sent to a third-party AI service.

Please report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
Earlier repository history contained deployment credentials that were removed
from the current tree without rewriting history. If you deployed from an older
commit, rotate those credentials immediately; details are in `SECURITY.md`.

## License

MIT. Copyright (c) 2026 Sutra contributors. See [LICENSE](LICENSE) and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
