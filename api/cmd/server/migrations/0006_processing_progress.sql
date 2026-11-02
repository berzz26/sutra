ALTER TABLE video_processing
    ADD COLUMN stage TEXT NOT NULL DEFAULT 'queued',
    ADD COLUMN progress_percent INTEGER NOT NULL DEFAULT 0
        CHECK (progress_percent BETWEEN 0 AND 100),
    ADD COLUMN cue_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN embedding_count INTEGER NOT NULL DEFAULT 0;
