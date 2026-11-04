# Troubleshooting

Start with service status and logs:

```sh
docker compose ps
docker compose logs --no-color model-init api frontend db
```

If you changed configuration, validate it first:

```sh
docker compose config --quiet
docker compose --profile artifacts config --quiet
```

## Startup and Compose

### `docker compose up` fails immediately

Check the error text. Common causes:

- Docker daemon is not running.
- Compose is older than v2.20 and does not support
  `depends_on.condition: service_completed_successfully`.
- The host is ARM64; the pinned images and binaries are amd64 only.
- A port is already in use.

Check versions:

```sh
docker version
docker compose version
uname -m    # must be x86_64 for the provided images
```

### Port 3000 or 8080 is already in use

Change the host ports in `.env`:

```sh
FRONTEND_PORT=3100
API_PORT=8180
```

Then:

```sh
docker compose up -d
```

The frontend URL becomes <http://localhost:3100>. The API is only bound to
loopback.

### `model-init` exits with a checksum or size mismatch

The service deletes the bad file and fails rather than letting the API load it.
Possible causes: interrupted download, a proxy that rewrites content, or a
manifest/artifact mismatch.

Inspect logs:

```sh
docker compose logs model-init
```

Retry:

```sh
docker compose up model-init
```

If it keeps failing, remove the model volume and retry:

```sh
docker compose down
docker volume rm sutra_model-data
docker compose up --build -d
```

Do not disable checksum verification or manually place a file that does not
match `deploy/model-init/model-manifest.json`.

### `model-init` cannot reach Hugging Face

The downloader needs HTTPS access to `huggingface.co`. Corporate proxies,
egress firewalls, and DNS restrictions can block it. Either allow the domain,
configure Docker's proxy, or follow the offline model preload steps in
[Installation](installation.md#offline-or-air-gapped-installs).

### API image build fails downloading ONNX Runtime, yt-dlp, or Deno

The API Dockerfile downloads from `github.com` releases and verifies SHA-256.
Failures are usually transient network errors or a changed upstream asset.
Retry:

```sh
docker compose build --no-cache api
```

If the checksum does not match, do not bypass it. Verify the pinned version and
URLs against <https://github.com/microsoft/onnxruntime/releases>,
<https://github.com/yt-dlp/yt-dlp/releases>, and
<https://github.com/denoland/deno/releases>. Update the Dockerfile, manifest,
and `THIRD_PARTY_NOTICES.md` together.

### API exits at startup

Read the first fatal log line:

```sh
docker compose logs api | head -50
```

Common messages:

| Message | Likely cause |
|---|---|
| `config: DATABASE_URL is required` | Empty `DATABASE_URL` and no `POSTGRES_*` defaults available. |
| `embedding: artifact ... no such file` | Model volume not initialized; `model-init` failed or was skipped. |
| `initialize ONNX runtime` | Wrong `ONNXRUNTIME_SHARED_LIBRARY_PATH`, missing shared library, or wrong architecture. |
| `expected token output dimension 384` | Model does not match `EMBEDDING_DIMENSIONS`. |
| `pgvector is required` | Database lacks the `vector` extension or permission to create it. |
| `config: invalid integer for ...` | Bad value in `.env`. |

### API restarts repeatedly

`restart: unless-stopped` hides the container exit in `docker compose ps`.
Inspect the logs and the exit code:

```sh
docker compose logs --tail=100 api
docker inspect --format '{{.State.ExitCode}}' sutra-api-1
```

Container names may differ if `COMPOSE_PROJECT_NAME` is set; run
`docker compose ps -a` to see actual names.

### `readyz` returns 503

```sh
curl -i http://localhost:8080/readyz
docker compose logs db
docker compose exec db sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

Causes include a still-initializing database, wrong credentials, the database
container being stopped, or a full disk. The API's liveness (`/healthz`) can be
OK while readiness fails.

### The frontend loads but API calls fail with 502/503

The nginx container proxies `/api/` to `http://api:8080`. Verify:

```sh
curl -i http://localhost:3000/api/v1/health
docker compose logs api frontend
docker compose ps
```

If the API is unhealthy, the frontend may still serve static files. Fix the API
first. A 502 from nginx with `api could not be resolved` usually means the
Compose network/service is not up.

### Browser shows CORS errors

Sutra expects same-origin access through nginx. If you call the API directly
from a different origin, the API does not add CORS headers. Serve both through
the same reverse proxy, or add CORS at your gateway. Do not add permissive CORS
to a shared instance.

## Ingestion

### `YTDLP_NOT_INSTALLED` on host runs

The API image includes yt-dlp. Host development requires it on `PATH`:

```sh
yt-dlp --version
```

Install a recent yt-dlp release and rerun. For YouTube challenge solving, install
Deno as well. Uploaded transcripts do not need either tool.

### `YTDLP_TIMEOUT`

The default per-command timeout is 120 seconds. Slow networks or large metadata
responses can exceed it. Increase `YTDLP_TIMEOUT_SECONDS` cautiously, or retry
later. Check API logs for the command that timed out.

### `YTDLP_RATE_LIMITED` (HTTP 429)

YouTube is rate limiting the host. Wait, lower
`TRANSCRIPT_MAX_CONCURRENT_INGESTIONS`, and retry. Sutra does not expose cookie
or proxy configuration, so it cannot bypass login/rate restrictions.

### `VIDEO_INACCESSIBLE`

Private, members-only, age-restricted, or login-required videos cannot be
fetched. This is intentional; there is no cookie import path.

### `NO_TRANSCRIPT` or `LANGUAGE_UNAVAILABLE`

Use the discovery endpoint to see what is actually available:

```sh
curl 'http://localhost:3000/api/v1/videos/subtitles?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DVIDEO_ID'
```

Then submit the exact `language` key and `source`. Omitting `language` allows
the default English/manual/original selection. A requested language never
silently falls back to a different language.

### Upload returns `UNSUPPORTED_FILE_TYPE` or `INVALID_SUBTITLE_FILE`

- Only `.vtt` and `.srt` are accepted.
- The extension must match the content; an SRT file renamed to `.vtt` is
  rejected.
- Empty or malformed transcripts are rejected.
- Check the size against `TRANSCRIPT_MAX_UPLOAD_MB`.

### Upload returns `FILE_TOO_LARGE`

Increase `TRANSCRIPT_MAX_UPLOAD_MB` and restart the API. The Go server's body
limit is derived from this value.

### Upload is deduplicated unexpectedly

Deduplication is by SHA-256 of the file bytes, not filename. Identical content
returns the existing video. Change the content or delete the existing video if
you need a separate row.

## Indexing

### Processing stays at `pending` or `processing`

Check workers and jobs:

```sh
docker compose logs -f api
```

```sql
SELECT id, video_id, status, attempts, last_error, run_after
FROM jobs ORDER BY created_at DESC LIMIT 20;

SELECT video_id, status, stage, progress_percent, last_error
FROM video_processing ORDER BY updated_at DESC LIMIT 20;
```

Causes:

- The API container is not running or restarted during processing.
- The job is waiting on `run_after` after a retryable failure.
- The embedding timeout (`EMBEDDING_TIMEOUT`, default 5m) is too short.
- The host is out of memory.

### Processing status is `failed`

`last_error` contains the cause. Common causes are a missing/corrupt model,
ONNX runtime failure, database connection loss, or context timeout. After fixing
the cause, retry:

```sh
curl -X POST http://localhost:3000/api/v1/videos/VIDEO_ID/process
```

Or insert a new `embed` job (the partial unique index prevents duplicates of an
active job).

### Search returns no results

1. Confirm the video's processing status is `completed`.
2. Confirm the library contains the video: `GET /api/v1/videos`.
3. Try a simpler or more distinctive query.
4. Remove `video_id` to search the whole library.
5. Check that the chunks table is populated:

   ```sql
   SELECT video_id, count(*), count(embedding)
   FROM transcript_chunks GROUP BY video_id;
   ```

A failed or unfinished index will not produce results.

### Search returns `SEARCH_FAILED`

Check API logs. Causes include query embedding failure, ONNX session issues,
database timeouts, or exceeding `SEARCH_TIMEOUT`. Increase the timeout only
after confirming the model and database are healthy.

## Database and storage

### `pgvector is required`

Use the provided `pgvector/pgvector` image, or install pgvector in an external
PostgreSQL and grant the role permission to create the extension:

```sql
CREATE EXTENSION IF NOT EXISTS vector;
```

Sutra fails migrations rather than silently using text-only search.

### Disk full

Embeddings and indexes consume disk. Inspect:

```sh
docker system df
docker compose exec db df -h /var/lib/postgresql/data
```

Back up and prune old data deliberately. Do not delete the database volume to
free space unless you intend to lose the library.

### Permission errors on volumes

The API runs as UID 10001. Named volumes created by Docker are normally owned
appropriately. If you bind-mount host directories instead of named volumes,
ensure the container user can read the model/runtime files and write temp
directories. The provided Compose file uses named volumes and does not need
host permission changes.

## Clean reset

To test from scratch, including the model cache:

```sh
docker compose down -v
docker compose up --build -d
```

This deletes all videos, transcripts, embeddings, jobs, optional artifacts, and
the cached model. It is intentionally destructive.

## Security incident

If you deployed an older commit and suspect credential exposure, see
[SECURITY.md](../SECURITY.md). Rotate database and object-store credentials
immediately; changing `.env` alone does not invalidate credentials that were
already used by a running deployment.
