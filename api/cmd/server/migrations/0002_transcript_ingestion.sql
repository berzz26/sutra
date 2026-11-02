-- 0002_transcript_ingestion.sql: schema for transcript ingestion.

-- Extended video metadata gathered from yt-dlp (nullable: uploads may lack any
-- of these) and lifecycle statuses that reflect the synchronous ingestion flow.
ALTER TABLE videos
    ADD COLUMN original_url  TEXT,
    ADD COLUMN thumbnail_url TEXT,
    ADD COLUMN duration_ms   BIGINT,
    ADD COLUMN channel       TEXT,
    ADD COLUMN language      TEXT;

ALTER TABLE videos DROP CONSTRAINT videos_status_check;
ALTER TABLE videos ADD CONSTRAINT videos_status_check
    CHECK (status IN ('pending', 'fetching_transcript', 'processing', 'ready', 'failed', 'no_transcript'));

-- One normalized transcript per video. Cues continue to live in
-- transcript_cues, keyed by video_id (unchanged from 0001).
CREATE TABLE transcripts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id          UUID NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    -- manual_subtitle | auto_generated | direct_upload
    extraction_method TEXT NOT NULL CHECK (extraction_method IN ('manual_subtitle', 'auto_generated', 'direct_upload')),
    -- direct uploads keep their uploaded format; YouTube extraction is normalized to vtt
    original_format   TEXT NOT NULL CHECK (original_format IN ('vtt', 'srt')),
    language          TEXT,
    -- object key of the original subtitle artifact in the artifact store; NULL when artifacts are disabled
    artifact_key      TEXT,
    cue_count         INTEGER NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (video_id)
);

CREATE INDEX idx_transcripts_video ON transcripts (video_id);
