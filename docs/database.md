# Database

Sutra uses PostgreSQL 16 with the [pgvector](https://github.com/pgvector/pgvector)
extension. The Compose stack uses `pgvector/pgvector:0.8.7-pg16-bookworm`. There
is no ORM: queries are written with `pgx` and live in `api/internal/persistence`.

## Automatic migrations

SQL migrations are embedded in the API binary from
`api/cmd/server/migrations/*.sql` and applied at startup, in filename order.
Each migration runs in its own transaction and is recorded by filename in
`schema_migrations`:

```sql
CREATE TABLE schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Migrations are forward-only. There is no down-migration command. Back up before
upgrading data you care about.

If pgvector is unavailable, startup fails with a clear error rather than
silently degrading search. The database role must be able to create the
`vector` extension (`CREATE EXTENSION IF NOT EXISTS vector`), or the extension
must already be installed.

## Tables

### `videos`

One row per YouTube video or uploaded transcript.

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Primary key, `gen_random_uuid()`. |
| `source_type` | `TEXT` | `youtube` or `upload`. |
| `source_ref` | `TEXT` | YouTube video ID, or SHA-256 of an uploaded file. |
| `title` | `TEXT` | Nullable for unusual extraction results. |
| `status` | `TEXT` | Lifecycle status; see below. |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | Timestamps. |
| `original_url` | `TEXT` | Canonical YouTube URL or an upload-provided reference. |
| `thumbnail_url` | `TEXT` | YouTube thumbnail, when available. |
| `duration_ms` | `BIGINT` | Duration in milliseconds, when known. |
| `channel` | `TEXT` | Channel/uploader, when known. |
| `language` | `TEXT` | Selected subtitle language or upload-provided language. |

Constraints and indexes:

- `UNIQUE (source_type, source_ref)` provides deduplication.
- `idx_videos_status` on `status`.
- Status check: `pending`, `fetching_transcript`, `processing`, `ready`,
  `failed`, `no_transcript`.

### `transcripts`

One summary row per video.

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Primary key. |
| `video_id` | `UUID` | `REFERENCES videos(id) ON DELETE CASCADE`, unique. |
| `extraction_method` | `TEXT` | `manual_subtitle`, `auto_generated`, or `direct_upload`. |
| `original_format` | `TEXT` | `vtt` or `srt`. |
| `language` | `TEXT` | Nullable. |
| `artifact_key` | `TEXT` | Object key when artifact storage is enabled; otherwise null. |
| `cue_count` | `INTEGER` | Number of raw cues. |
| `created_at` | `TIMESTAMPTZ` | Timestamp. |

### `transcript_cues`

The raw, timestamped subtitle cues exactly as extracted or uploaded. This is the
source of truth for transcript text. Processing never rewrites these rows.

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Primary key. |
| `video_id` | `UUID` | Cascade delete with the video. |
| `seq` | `INTEGER` | Order within the video. |
| `start_ms`, `end_ms` | `BIGINT` | Timing in milliseconds. |
| `text` | `TEXT` | Cue text. |

`UNIQUE (video_id, seq)` plus `idx_transcript_cues_video (video_id, seq)`.

### `transcript_chunks`

Derived search units. A chunk may span several complete cues. Chunks carry the
384-dimensional embedding and a generated full-text vector.

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Deterministic UUID derived from video/version/sequence/timing/text. |
| `video_id` | `UUID` | Cascade delete with the video. |
| `seq` | `INTEGER` | Chunk order. Unique per video. |
| `start_ms`, `end_ms` | `BIGINT` | Span of the included cues. |
| `text` | `TEXT` | Joined cue text. |
| `word_count` | `INTEGER` | Word count after normalization. |
| `processing_version` | `TEXT` | Chunking version (`v1` currently). |
| `embedding_model` | `TEXT` | Model/version, for example `BAAI/bge-small-en-v1.5/v1.5`. |
| `embedding` | `vector(384)` | Nullable during migration/backfill. |
| `search_vector` | `tsvector` | Generated `to_tsvector('english', coalesce(text, ''))`, stored. |
| `created_at` | `TIMESTAMPTZ` | Timestamp. |

Indexes:

- `UNIQUE (video_id, seq)`.
- `idx_transcript_chunks_video (video_id, start_ms)`.
- `idx_transcript_chunks_search_vector` — GIN on `search_vector`.
- `idx_transcript_chunks_embedding_hnsw` — HNSW cosine index on `embedding`,
  partial `WHERE embedding IS NOT NULL`.

On small datasets PostgreSQL may correctly choose a sequential scan over HNSW.
The index is still used as the dataset grows.

### `video_processing`

Per-video processing state, exposed through the processing endpoints.

| Column | Type | Notes |
|---|---|---|
| `video_id` | `UUID` | Primary key, cascade delete. |
| `status` | `TEXT` | `pending`, `processing`, `failed`, or `completed`. |
| `processing_version`, `model_version` | `TEXT` | Used for idempotency checks. |
| `last_error` | `TEXT` | Last failure message, cleared on success. |
| `chunk_count` | `INTEGER` | Number of indexed chunks. |
| `stage` | `TEXT` | Progress stage such as `chunking`, `embedding`, `saving`. |
| `progress_percent` | `INTEGER` | 0–100. |
| `cue_count`, `embedding_count` | `INTEGER` | Progress counters. |
| `started_at`, `completed_at`, `updated_at` | `TIMESTAMPTZ` | Timestamps. |

### `jobs`

PostgreSQL-backed queue for embedding work. There is no Redis or external
broker.

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Primary key. |
| `video_id` | `UUID` | Cascade delete. |
| `kind` | `TEXT` | Constrained to `ingest_youtube`, `ingest_upload`, `embed`, `index`; only `embed` is currently produced. |
| `status` | `TEXT` | `queued`, `running`, `succeeded`, `failed`. |
| `attempts` | `INTEGER` | Incremented when a job is claimed. |
| `last_error` | `TEXT` | Last failure message. |
| `run_after` | `TIMESTAMPTZ` | Retry scheduling. |
| `created_at`, `updated_at`, `started_at`, `finished_at` | `TIMESTAMPTZ` | Timestamps. |

A partial unique index ensures at most one active (`queued` or `running`) embed
job per video:

```sql
CREATE UNIQUE INDEX idx_jobs_one_active_embedding_per_video
    ON jobs(video_id, kind)
    WHERE kind = 'embed' AND status IN ('queued', 'running');
```

## Lifecycle

### Video status

```text
pending -> fetching_transcript -> ready
                             \-> no_transcript
                             \-> failed
pending -> processing -> ready
                      \-> failed
```

- `ready` and `no_transcript` are terminal for ingestion idempotency. New
  requests return the existing row.
- `failed` videos can be retried by submitting the same source again.
- In-flight statuses return the existing row instead of starting a second
  extraction.

Uploads go `pending -> processing -> ready|failed`. YouTube ingestion downloads
and persists subtitles synchronously, so its status is updated around the fetch
and finalize steps. Embedding is asynchronous in both cases.

### Processing status

- `pending`: state row created but processing has not started.
- `processing`: chunking/embedding/saving in progress.
- `completed`: chunks and embeddings committed.
- `failed`: last error recorded; a retry can re-run the job.

A completed video with the same processing version and model version is a no-op
when reprocessed.

### Job lifecycle

1. Ingestion inserts one `embed` job with `status='queued'`.
2. A worker claims a due job in a transaction with
   `FOR UPDATE SKIP LOCKED`, sets `status='running'`, and increments `attempts`.
3. Success sets `status='succeeded'`.
4. Failure sets `status='failed'` unless attempts remain, in which case the job
   returns to `queued` with `run_after` set.
5. Up to five attempts are made in total; backoff is exponential (2s, 4s, 8s,
   16s).

Workers run inside the API process. `PROCESSING_WORKERS` controls concurrency.
Chunk writes use an upsert on `(video_id, seq)` and delete stale higher sequence
numbers, so retries do not duplicate chunks.

## Deduplication and idempotency

| Source | Dedup key | Behavior on repeat |
|---|---|---|
| YouTube | Canonical YouTube video ID | Returns existing `ready`/`no_transcript` row; retries `failed` rows. |
| Upload | SHA-256 of file bytes | Returns existing video; identical bytes under a different filename still dedupe. |

YouTube URLs are canonicalized before lookup. Playlist URLs and non-YouTube URLs
are rejected. See [Ingestion](ingestion.md).

## Backfill

`go run ./cmd/backfill` inserts embed jobs for videos that have cues but no
chunks, and only when no active embed job already exists. It is safe to run
repeatedly and resume after interruption.

## Operational queries

```sql
-- Recent processing failures
SELECT video_id, status, stage, progress_percent, last_error, updated_at
FROM video_processing
WHERE status = 'failed'
ORDER BY updated_at DESC;

-- Queue depth
SELECT status, count(*)
FROM jobs
WHERE kind = 'embed'
GROUP BY status;

-- Indexed coverage
SELECT count(*) AS videos,
       count(*) FILTER (WHERE EXISTS (
           SELECT 1 FROM transcript_chunks c WHERE c.video_id = v.id
       )) AS indexed
FROM videos v;
```

Backup and restore procedures are in
[Backup and restore](backup-and-restore.md).
