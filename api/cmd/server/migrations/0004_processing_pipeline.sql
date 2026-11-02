-- 0004_processing_pipeline.sql: durable retry scheduling and vector retrieval index.

ALTER TABLE jobs
    ADD COLUMN run_after TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX idx_jobs_one_active_embedding_per_video
    ON jobs(video_id, kind)
    WHERE kind = 'embed' AND status IN ('queued', 'running');

-- HNSW gives good recall and predictable query latency for the expected
-- read-heavy workload. It costs more write time and disk than an exact scan;
-- the partial predicate keeps unembedded legacy rows out of the index.
CREATE INDEX idx_transcript_chunks_embedding_hnsw
    ON transcript_chunks USING hnsw (embedding vector_cosine_ops)
    WHERE embedding IS NOT NULL;
