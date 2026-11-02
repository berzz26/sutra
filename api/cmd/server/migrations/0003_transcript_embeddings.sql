CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE transcript_chunks
    ADD COLUMN seq INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN word_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN processing_version TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN embedding_model TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN embedding vector(384);

CREATE UNIQUE INDEX idx_transcript_chunks_video_seq ON transcript_chunks(video_id, seq);

CREATE TABLE video_processing (
    video_id UUID PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK(status IN ('pending','processing','failed','completed')),
    processing_version TEXT NOT NULL,
    model_version TEXT NOT NULL,
    last_error TEXT,
    chunk_count INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
