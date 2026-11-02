-- 0005_search_text.sql: PostgreSQL-native lexical search over derived chunks.

ALTER TABLE transcript_chunks
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(text, ''))) STORED;

CREATE INDEX idx_transcript_chunks_search_vector
    ON transcript_chunks USING gin (search_vector);
