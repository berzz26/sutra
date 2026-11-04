# Backup and restore

Sutra persists state in Docker named volumes. There are no automatic backups.

| Volume | Contents | Critical? |
|---|---|---|
| `sutra_postgres-data` | Videos, transcripts, cues, chunks, embeddings, jobs, migrations | **Yes** |
| `sutra_model-data` | Verified model and tokenizer | No; re-downloadable |
| `sutra_minio-data` | Original subtitle artifacts, only if the `artifacts` profile is used | Depends on your retention policy |

Named volumes survive `docker compose down` and container recreation. They are
deleted by `docker compose down -v`.

## Back up PostgreSQL

### Using the Makefile

With the stack running:

```sh
make db-backup
```

This runs `pg_dump -Fc` inside the `db` container and writes a timestamped
custom-format dump to `backups/`. The target prints the created path.

### Manual dump

```sh
mkdir -p backups
docker compose exec -T db sh -c \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > "backups/sutra-$(date -u +%Y%m%d-%H%M%S).dump"
```

`pg_dump` takes a consistent snapshot while the database is in use. For the
cleanest backup of a busy instance, stop the API first so no ingestion or
indexing writes are in flight:

```sh
docker compose stop api frontend
make db-backup
docker compose start api frontend
```

### Plain SQL dump

For portability or inspection:

```sh
docker compose exec -T db sh -c \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  > backups/sutra.sql
```

Custom format (`-Fc`) is preferred because it supports selective restore and
compression.

## Restore PostgreSQL

### Using the Makefile

Start the database (and only the database) first:

```sh
docker compose up -d db
make db-restore FILE=backups/sutra-20261103-120000.dump
docker compose up -d
```

The Makefile uses:

```sh
pg_restore --clean --if-exists --no-owner
```

So the target database must already exist. The dump restores schema, data,
extensions, indexes, and `schema_migrations`.

### Manual restore

```sh
docker compose up -d db
docker compose exec -T db sh -c \
  'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner' \
  < backups/sutra-20261103-120000.dump
docker compose up -d
```

If you restore into a brand-new volume and `pg_restore` reports errors about
existing objects, confirm you are restoring into the intended database and not
a partially initialized one. For a completely clean target:

```sh
docker compose down
docker volume rm sutra_postgres-data
docker compose up -d db
# restore, then start the rest
docker compose up -d
```

`docker volume rm` permanently deletes the old database. Make sure the dump is
valid and copied off the host first.

### Verify after restore

```sh
curl -fsS http://localhost:8080/readyz
curl -fsS http://localhost:3000/api/v1/videos
make smoke
```

Check migrations and indexed coverage:

```sh
docker compose exec db sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version;"'
```

## Back up optional artifacts

If `ARTIFACT_STORE_ENABLED=true` with the bundled MinIO profile, back up the
bucket separately. The database `artifact_key` references are useless without
the objects.

Using `mc` from a machine with network access to MinIO:

```sh
mc alias set sutra http://127.0.0.1:9000 sutra 'your-secret'
mc mirror --overwrite sutra/sutra-artifacts backups/minio/sutra-artifacts
```

Or archive the volume while MinIO is stopped:

```sh
docker compose --profile artifacts stop minio
docker run --rm \
  -v sutra_minio-data:/data:ro \
  -v "$PWD/backups:/backup" \
  alpine:3.20.3 tar czf /backup/minio-data.tar.gz -C /data .
docker compose --profile artifacts start minio
```

For an external S3 provider, use its versioning/replication/backup features.

## Back up the model volume

Not required. `model-init` verifies and re-downloads the pinned model to an
empty volume. If you need an offline copy, archive `sutra_model-data`:

```sh
docker run --rm \
  -v sutra_model-data:/models:ro \
  -v "$PWD/backups:/backup" \
  alpine:3.20.3 tar czf /backup/model-data.tar.gz -C /models .
```

Restore it before starting the stack, or let `model-init` re-download the
verified files. See [Models](models.md) for checksums.

## Upgrade and rollback

Recommended upgrade sequence:

```sh
make db-backup
git pull
docker compose up --build -d
docker compose ps
make smoke
```

Migrations are forward-only. There is no automated rollback. To roll back code,
check out the previous commit, rebuild, and restore the database from the dump
taken before the upgrade if the newer schema is incompatible with the older
binary.

## Disaster recovery checklist

1. Install Docker and clone the same or a compatible release.
2. Restore `sutra_postgres-data` from a `pg_dump` (preferred) or archive.
3. If artifacts were enabled, restore the object store/bucket.
4. Let `model-init` re-download the pinned model, or restore the model volume.
5. Start the stack: `docker compose up -d`.
6. Verify `/readyz`, `/api/v1/videos`, search, and `make smoke`.
7. Check that the database and artifact credentials in `.env` are correct.

Store dumps off the Docker host. A backup that exists only inside the volume it
protects is not a backup.
