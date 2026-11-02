-- 0001_init.sql: initial schema. UUID primary keys generated in the database
-- via gen_random_uuid (available natively since PG13).

CREATE TABLE videos (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- 'youtube' (source_ref = YouTube video ID) or 'upload' (source_ref = object key)
    source_type TEXT NOT NULL CHECK (source_type IN ('youtube', 'upload')),
    source_ref  TEXT NOT NULL,
    title       TEXT,
    -- pending -> processing -> ready / failed
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One indexed copy per source, either a YouTube video or a unique uploaded file.
    UNIQUE (source_type, source_ref)
);

CREATE INDEX idx_videos_status ON videos (status);

-- Raw transcript cues, exactly as extracted by yt-dlp or as uploaded.
-- Cues are the source of truth and are never rewritten by later processing.
CREATE TABLE transcript_cues (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    seq      INTEGER NOT NULL,
    start_ms BIGINT NOT NULL,
    end_ms   BIGINT NOT NULL,
    text     TEXT NOT NULL,

    UNIQUE (video_id, seq)
);

CREATE INDEX idx_transcript_cues_video ON transcript_cues (video_id, seq);

-- Indexed search chunks produced by later processing. Conceptually separate
-- from cues: one chunk may span several cues. Embeddings are added in a later
-- migration together with the pgvector extension.
CREATE TABLE transcript_chunks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id    UUID NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    start_ms    BIGINT NOT NULL,
    end_ms      BIGINT NOT NULL,
    text        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_transcript_chunks_video ON transcript_chunks (video_id, start_ms);

-- Background job queue (worker lives in the same process; no external broker).
CREATE TABLE jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id    UUID REFERENCES videos (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('ingest_youtube', 'ingest_upload', 'embed', 'index')),
    -- queued -> running -> succeeded / failed
    status      TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_jobs_status_kind ON jobs (status, kind);
CREATE INDEX idx_jobs_video ON jobs (video_id);
