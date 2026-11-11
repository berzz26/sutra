# Configuration

Sutra is configured with environment variables only. Docker Compose reads
values from your shell and from an optional `.env` file at the repository root.
Precedence is:

1. Values exported in your shell.
2. Values in `.env`.
3. Defaults in `docker-compose.yml` / the Go application.

No `.env` is required for the local stack. Start from [.env.example](../.env.example)
when you need to override something.

## Host ports and Compose

| Variable | Default | Used by | Description |
|---|---|---|---|
| `FRONTEND_PORT` | `3000` | Compose | Host port for the web UI. Published on all interfaces. |
| `API_PORT` | `8080` | Compose | Host port for direct API access. Bound to `127.0.0.1`. |
| `POSTGRES_PORT` | `5432` | dev Compose | Host port for PostgreSQL when the dev override is used. Bound to `127.0.0.1`. |
| `MINIO_API_PORT` | `9000` | dev Compose | Host port for the optional MinIO S3 API. Bound to `127.0.0.1`. |
| `MINIO_CONSOLE_PORT` | `9001` | dev Compose | Host port for the optional MinIO console. Bound to `127.0.0.1`. |
| `MINIO_IMAGE` | `quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z` | Compose | Image for the optional `artifacts` profile. Upstream community image availability is not guaranteed; use a maintained S3-compatible image if needed. |

The API and frontend containers always listen on port `8080` internally. The
frontend proxies `/api` to the API, so browsers normally only need port `3000`.

## PostgreSQL

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_USER` | `searcher` | Database role created by the PostgreSQL image. |
| `POSTGRES_PASSWORD` | `searcher` | Database password. **Change this for any non-local deployment.** |
| `POSTGRES_DB` | `searcher` | Database name. |
| `DATABASE_URL` | built from the values above | Full pgx connection string used by the API. Takes precedence over `POSTGRES_*` when set. |

The default connection string is:

```text
postgresql://searcher:searcher@db:5432/searcher
```

To use an external PostgreSQL server, it must have the `vector` extension
available and the API must be able to reach it. Set `DATABASE_URL` explicitly,
for example:

```sh
DATABASE_URL='postgresql://sutra:secret@postgres.internal:5432/sutra?sslmode=require'
```

The database container has no host port in the default stack. Use
`docker-compose.dev.yml` for localhost-only access from the host.

## API server

| Variable | Default | Description |
|---|---|---|
| `SERVER_HOST` | `0.0.0.0` | Listen address inside the container. |
| `SERVER_PORT` | `8080` | Listen port inside the container. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `SHUTDOWN_TIMEOUT_SECONDS` | `15` | Graceful shutdown budget for HTTP and workers. |
| `ADMIN_API_TOKEN` | empty | Static bearer token for the administrative endpoints: `GET /api/v1/videos` (global video listing) and `DELETE /api/v1/videos/:id` (video deletion). Sent as `Authorization: Bearer <token>` or the `X-Admin-Token` header. When unset, those endpoints reject every request (fail closed). The value is never logged and never sent to the frontend; generate one with e.g. `openssl rand -hex 32`. |

## Ingestion

| Variable | Default | Description |
|---|---|---|
| `YTDLP_BINARY` | `/usr/local/bin/yt-dlp` in Compose; `yt-dlp` for host runs | Path or name of the yt-dlp executable. |
| `YTDLP_TIMEOUT_SECONDS` | `120` | Timeout for each yt-dlp command. |
| `TRANSCRIPT_MAX_UPLOAD_MB` | `20` | Maximum uploaded VTT/SRT size in MiB. Must be positive. |
| `TRANSCRIPT_MAX_CONCURRENT_INGESTIONS` | `2` | Maximum concurrent YouTube extraction operations in one API process. |

The Fiber body limit is set to the upload limit plus 1 MiB of multipart
overhead.

## Embedding and processing

| Variable | Default | Description |
|---|---|---|
| `EMBEDDING_MODEL` | `BAAI/bge-small-en-v1.5` | Model identifier stored with processed chunks and included in the processing version. |
| `EMBEDDING_MODEL_VERSION` | `v1.5` | Model version stored with processed chunks. |
| `EMBEDDING_ONNX_PATH` | `/models/bge-small-en-v1.5/model.onnx` in Compose | ONNX model path. |
| `EMBEDDING_TOKENIZER_PATH` | `/models/bge-small-en-v1.5/tokenizer.json` in Compose | Hugging Face `tokenizer.json` path. |
| `ONNXRUNTIME_SHARED_LIBRARY_PATH` | `/opt/onnxruntime/lib/libonnxruntime.so` in Compose | ONNX Runtime shared library loaded through cgo/dlopen. |
| `EMBEDDING_DIMENSIONS` | `384` | Expected model output dimension. Must match the pgvector column. |
| `EMBEDDING_BATCH_SIZE` | `32` | Texts embedded per ONNX inference batch. |
| `EMBEDDING_MAX_TOKENS` | `512` | Token truncation limit. Must be between 1 and 512. |
| `EMBEDDING_ONNX_THREADS` | `4` | Intra-op ONNX Runtime thread count. |
| `EMBEDDING_TIMEOUT` | `5m` | Go duration limit for processing one video. |
| `PROCESSING_WORKERS` | `2` | Number of in-process PostgreSQL job workers. |
| `MODEL_REVISION` | empty | Optional guard for `model-init`. When set, it must exactly match the revision pinned in `deploy/model-init/model-manifest.json`. |

When running the API directly on the host (outside Docker), the default
embedding paths are relative to the `api/` directory:

```text
embedding/models/bge-small-en-v1.5/model.onnx
embedding/models/bge-small-en-v1.5/tokenizer.json
third_party/onnxruntime/onnxruntime-linux-x64-1.31.1/lib/libonnxruntime.so
```

`make dev-deps` downloads the ignored model weights and ONNX Runtime distribution
into those locations.

### Fixed chunking constants

Chunking parameters are compiled into the processing pipeline, not read from the
environment:

| Constant | Value |
|---|---|
| Target chunk size | 180 words |
| Minimum chunk size | 80 words |
| Maximum chunk size | 250 words |
| Overlap | up to 30 words, using complete cues |
| Processing version | `v1` |

Changing these values requires a code change and a reindex. See
[Models](models.md#changing-the-model-or-chunking).

## Search

| Variable | Default | Description |
|---|---|---|
| `SEARCH_CANDIDATE_POOL` | `100` | Candidates retrieved independently from vector and lexical search. Must be positive. |
| `SEARCH_RRF_K` | `60` | Reciprocal Rank Fusion smoothing constant. |
| `SEARCH_TIMEOUT` | `10s` | Timeout covering query embedding plus both database candidate queries. |

`SEARCH_CANDIDATE_POOL` and `SEARCH_RRF_K` must be positive integers.

The public search endpoint separately enforces a query length of 2,000
characters, a default `limit` of 10, and a maximum `limit` of 50.

## Optional artifact store

Artifact storage retains the original uploaded or downloaded subtitle file.
Structured cues, chunks, and embeddings always live in PostgreSQL, so the
application is fully functional without an artifact store.

| Variable | Default | Description |
|---|---|---|
| `ARTIFACT_STORE_ENABLED` | `false` | Set to `true` to require the configured S3-compatible store. |
| `MINIO_ENDPOINT` | empty | S3 endpoint, for example `minio:9000` or `s3.example.com`. Required when artifacts are enabled. |
| `MINIO_ACCESS_KEY` | `sutra` | Access key. |
| `MINIO_SECRET_KEY` | `sutra-local-only-change-me` | Secret key. **Change for non-local use.** |
| `MINIO_BUCKET` | `sutra-artifacts` | Bucket name. Required when artifacts are enabled. The bucket must exist. |
| `MINIO_REGION` | `us-east-1` | S3 region. |
| `MINIO_USE_SSL` | `false` | Use TLS for the S3 connection. |
| `MINIO_ROOT_USER` | `sutra` | Root user for the optional bundled MinIO container only. |
| `MINIO_ROOT_PASSWORD` | `sutra-local-only-change-me` | Root password for the optional bundled MinIO container only. |

To use the optional bundled MinIO service:

```sh
# In .env
ARTIFACT_STORE_ENABLED=true
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=sutra
MINIO_SECRET_KEY=sutra-local-only-change-me
MINIO_BUCKET=sutra-artifacts

# Start the profile
docker compose --profile artifacts up -d
```

The application does not create the bucket. Create it with the MinIO console,
`mc`, or your S3 provider before enabling artifacts. The bundled profile is
provided for convenience and is not required or automatically verified by CI.

## Frontend build variables

| Variable | Default | Description |
|---|---|---|
| `VITE_API_BASE_URL` | empty | API base URL baked into the frontend build. Empty means same-origin `/api/...`, which is the recommended production mode behind nginx. |
| `VITE_API_PROXY_TARGET` | `http://localhost:8080` | Vite dev/preview proxy target. Not used by the production nginx image. |

In Docker, the frontend build receives `VITE_API_BASE_URL` as a build argument.
Changing it requires rebuilding the frontend image.

## Validation behavior

The API refuses to start with invalid configuration. Examples:

- `DATABASE_URL` is missing.
- A boolean variable is not parseable.
- A positive integer requirement is not met.
- `EMBEDDING_MAX_TOKENS` is greater than 512.
- `EMBEDDING_TIMEOUT` or `SEARCH_TIMEOUT` is not a valid Go duration.
- `ARTIFACT_STORE_ENABLED=true` without `MINIO_ENDPOINT` or `MINIO_BUCKET`.

Check the API logs for the exact message:

```sh
docker compose logs api
```

## Security-related defaults

- PostgreSQL and MinIO have no host ports in the default stack.
- The API binds to `127.0.0.1` on the host.
- Containers run with `no-new-privileges`; the API runs as UID 10001.
- The default credentials are for local development only.
- There is no application authentication or TLS. Use a reverse proxy.

### Browser-local video libraries

The UI keeps each browser's video references in `localStorage`
(`sutra.videoLibrary.v1`). The shared database still holds every indexed
video, but the normal frontend only lists and searches the videos saved in
the current browser:

- The global listing endpoint (`GET /api/v1/videos`) and video deletion
  (`DELETE /api/v1/videos/:id`) require `ADMIN_API_TOKEN`.
- Entire Library searches always carry an explicit `video_ids` list; an
  empty list is a real empty scope and returns zero results — it is never
  treated as "search everything".
- Removing a video in the UI only deletes the local reference; the indexed
  database content and other browsers are unaffected. Clearing browser
  storage likewise removes local references without deleting any indexed
  content.

This is browser-level organization, not user authentication or strict
multi-user data isolation: any browser that knows a video's ID can still
reference it.
