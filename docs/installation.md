# Installation

This guide covers a clean local install, the first-run checks, upgrades, and the
offline/air-gapped path.

## Supported platform

| Item | Requirement |
|---|---|
| CPU architecture | Linux **amd64** (x86_64) |
| Container runtime | Docker Engine 24+ with Compose v2.20+ |
| Memory | 2 GB minimum, 4 GB recommended |
| Disk | ~4 GB free (images, model, database growth) |
| Network | Required on first run |

The API image contains x86_64 builds of ONNX Runtime, yt-dlp, and Deno. ARM64
hosts are not supported by the provided images. See
[Current limitations](../README.md#current-limitations).

## 1. Clone the repository

```sh
git clone https://github.com/berzz26/searcher.git
cd searcher
```

`master` is the default branch.

## 2. Start the stack

```sh
docker compose up --build -d
```

That single command:

1. Pulls PostgreSQL 16 with pgvector, Node, Go, Debian, Alpine, and nginx base
   images.
2. Builds the API image and verifies the pinned ONNX Runtime, yt-dlp, and Deno
   downloads with SHA-256 checksums.
3. Builds the frontend static bundle and serves it through an unprivileged
   nginx container.
4. Starts PostgreSQL and waits for its health check.
5. Runs `model-init`, which downloads the pinned BGE-small-en-v1.5 model and
   tokenizer into the `sutra_model-data` volume, verifies their checksums and
   sizes, and writes a `.ready` marker.
6. Starts the API only after PostgreSQL is healthy and `model-init` completed
   successfully. The API applies embedded migrations, loads the ONNX model,
   verifies it with a warm-up inference, and starts listening.
7. Starts the frontend only after the API is healthy.

No `.env` file and no manual database, model, or runtime installation is
required.

### Optional configuration before the first start

```sh
cp .env.example .env
# edit ports, credentials, or tuning values
```

The default stack is designed for local use. At minimum, change
`POSTGRES_PASSWORD` before exposing the stack to anyone else. See
[Configuration](configuration.md).

## 3. Wait for health

```sh
docker compose ps
```

`db`, `api`, and `frontend` should report `healthy`. `model-init` should show
`exited (0)`; it is a one-shot service, not a long-running one.

Follow startup if anything is still pending:

```sh
docker compose logs -f model-init api
```

The first build can take several minutes depending on network speed. The model
download is about 127 MiB and is cached in a named volume on later starts.

## 4. Open the application

| Service | URL | Notes |
|---|---|---|
| Web UI | <http://localhost:3000> | Main entry point |
| API via UI proxy | <http://localhost:3000/api/v1/...> | Same-origin path used by the browser |
| API direct | <http://localhost:8080> | Bound to `127.0.0.1` only |
| PostgreSQL | `db:5432` inside Compose | No host port by default |
| MinIO profile | `minio:9000` inside Compose | Optional, not started by default |

## 5. Verify the install

Basic checks:

```sh
curl -fsS http://localhost:3000/healthz
curl -fsS http://localhost:3000/api/v1/health
curl -fsS http://localhost:8080/readyz
```

Expected responses:

```text
ok
{"status":"ok"}
{"status":"ready"}
```

The first is nginx's plain-text liveness response; the other two are JSON from
the API.

`/healthz` and `/api/v1/health` are liveness probes. `/readyz` checks the
PostgreSQL connection and returns HTTP 503 with
`{"status":"unavailable","error":"database unreachable"}` when the database is
not usable.

End-to-end smoke test:

```sh
make smoke
```

`scripts/smoke-test.sh` uses the real HTTP API: it uploads
`scripts/fixtures/sample.vtt`, waits for background indexing to complete, and
runs a hybrid search restricted to the uploaded video. It exits non-zero on
failure. To test the direct API instead of the nginx proxy:

```sh
BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh
```

## Ports and host exposure

The default Compose file intentionally limits exposure:

- Frontend: `0.0.0.0:3000` (configurable with `FRONTEND_PORT`).
- API: `127.0.0.1:8080` (configurable with `API_PORT`).
- PostgreSQL: no host port.
- MinIO: no host port and not started unless the `artifacts` profile is used.

For host access to PostgreSQL or MinIO during development, use the dev
override, which publishes them on loopback only:

```sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d db
```

## Stop, restart, and reset

```sh
docker compose stop          # stop containers, keep them and volumes
docker compose down          # remove containers/network, keep named volumes
docker compose down -v       # remove containers AND all persisted data
```

Named volumes:

| Volume | Contents | Safe to delete? |
|---|---|---|
| `sutra_postgres-data` | Videos, cues, chunks, embeddings, jobs | Only if you want to lose all data |
| `sutra_model-data` | Verified ONNX model and tokenizer | Yes; it is re-downloaded |
| `sutra_minio-data` | Optional artifact objects | Only if artifact storage is enabled |

`docker compose down` is non-destructive. `docker compose down -v` permanently
deletes the database. Back up first if the data matters; see
[Backup and restore](backup-and-restore.md).

## Upgrade

```sh
git pull
docker compose up --build -d
```

Database migrations are embedded in the API binary and applied automatically at
startup. Migrations are forward-only; there is no down-migration command.
Back up the database before upgrading a deployment that holds important data.

## Offline or air-gapped installs

The default first run needs network access to Docker Hub/Quay/GitHub Container
Registry, GitHub Releases (ONNX Runtime, yt-dlp, Deno), and Hugging Face (model).
To install without internet:

1. On a connected machine with the same CPU architecture, build the images:

   ```sh
   docker compose build
   ```

2. Save the images (Compose names them after the project and service):

   ```sh
   docker save sutra-db sutra-model-init sutra-api sutra-frontend \
     | gzip > sutra-images.tar.gz
   ```

   Include the nginx and PostgreSQL base images only if your target host does
   not already have them. The simplest reliable approach is to save every image
   listed by `docker image ls` after the build.

3. On the target host, load the images:

   ```sh
   gunzip -c sutra-images.tar.gz | docker load
   ```

4. Pre-populate the model volume with the two verified files. Replace the host
   paths with your local copies:

   ```sh
   docker volume create sutra_model-data
   docker run --rm \
     -v sutra_model-data:/models \
     -v "$PWD/model.onnx:/src/model.onnx:ro" \
     -v "$PWD/tokenizer.json:/src/tokenizer.json:ro" \
     alpine:3.20.3 sh -c \
     'mkdir -p /models/bge-small-en-v1.5 && cp /src/model.onnx /src/tokenizer.json /models/bge-small-en-v1.5/'
   ```

   Verify the files against the checksums in
   `deploy/model-init/model-manifest.json`. `model-init` will verify them again
   and skip the download when they match.

5. Start without building or pulling:

   ```sh
   docker compose up -d --no-build
   ```

If the target host already has different image tags, set the relevant image
variables or retag the loaded images before starting. The model files are MIT
licensed; see [Models](models.md) for source URLs and checksums.

## Production notes

Sutra is a single-node application without built-in authentication or TLS. A
reasonable deployment shape is:

```text
Internet -> TLS reverse proxy with authentication -> frontend:3000
                                                     frontend -> api:8080
                                                     api -> db:5432
```

Recommendations:

- Set a strong, unique `POSTGRES_PASSWORD`; the default is local-only.
- Do not publish the API or database to untrusted networks.
- Put the stack behind Caddy, nginx, Traefik, or a cloud load balancer with TLS.
- Add authentication at the proxy (basic auth, OIDC, VPN, or an SSO gateway).
- Configure backups and monitor disk usage; embeddings and indexes grow with
  the library.
- Keep Docker and the pinned dependencies updated through the normal
  maintenance process.
- Review [SECURITY.md](../SECURITY.md), including the historical credential
  notice.

The Compose file does not set CPU or memory limits. On a shared host, add
resource limits with a Compose override appropriate for your environment.

## Next steps

- [Configuration](configuration.md) — ports, credentials, tuning
- [Ingestion](ingestion.md) — YouTube and upload behavior
- [Search](search.md) — result semantics
- [Troubleshooting](troubleshooting.md) — startup and extraction failures
