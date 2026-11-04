# Search

Search is hybrid: a local embedding model finds semantically related transcript
chunks, PostgreSQL full-text search finds exact terms, and Reciprocal Rank Fusion
(RRF) merges the two rankings.

```text
query
  ├─ BGE-small-en-v1.5 embedding ──> pgvector cosine candidates ─┐
  └─ websearch_to_tsquery('english') ──> GIN lexical candidates ─┤
                                                                 ├─ RRF ─> top N
                                                                 ┘
```

There is no external AI API, no answer generation, and no reranking step.

## Endpoint

```http
POST /api/v1/search
Content-Type: application/json
```

Request body:

```json
{
  "query": "How does database replication work?",
  "limit": 10,
  "offset": 0,
  "video_id": "optional-video-uuid"
}
```

| Field | Required | Rules |
|---|---|---|
| `query` | Yes | Non-empty after trimming; at most 2,000 characters (Unicode code points). |
| `limit` | No | Defaults to 10. Must be 1–50. |
| `offset` | No | Defaults to 0. Must be zero or greater. |
| `video_id` | No | Must be an existing video UUID; restricts results to that video. |

An unknown `video_id` returns HTTP 404. A malformed UUID or invalid limit/offset
returns HTTP 400 with code `INVALID_SEARCH`.

## Response

```json
{
  "query": "How does database replication work?",
  "limit": 10,
  "offset": 0,
  "duration_ms": 14,
  "results": [
    {
      "chunk_id": "1da863ed-bfbd-51ed-85ef-1aef5fa67775",
      "video_id": "9d5e...",
      "video_title": "Distributed systems lecture",
      "original_url": "https://www.youtube.com/watch?v=VIDEO_ID",
      "timestamp_url": "https://www.youtube.com/watch?v=VIDEO_ID&t=754",
      "text": "A replica applies the primary's write-ahead log ...",
      "start_ms": 754000,
      "end_ms": 812000,
      "score": 0.03278688524590164,
      "semantic_score": 0.8123,
      "lexical_score": 0.0601,
      "semantic_rank": 2,
      "lexical_rank": 4
    }
  ]
}
```

Fields:

| Field | Meaning |
|---|---|
| `duration_ms` | Server-side time covering query embedding and both candidate queries. |
| `chunk_id` | Deterministic transcript chunk identifier. |
| `video_id` / `video_title` | Source video. |
| `original_url` | Original YouTube URL or an upload-provided reference, when present. |
| `timestamp_url` | For a valid original URL: the URL with `t=<start seconds>` added. Existing query parameters are preserved. |
| `text` | Matched chunk text (one or more complete cues). |
| `start_ms`, `end_ms` | Chunk timing in milliseconds. |
| `score` | **RRF fusion score**, not a probability or cosine similarity. |
| `semantic_score` | Cosine similarity (`1 - pgvector cosine distance`), present when vector search returned the chunk. |
| `lexical_score` | PostgreSQL `ts_rank_cd`, present when lexical search returned the chunk. |
| `semantic_rank`, `lexical_rank` | 1-based rank in each candidate list, present when that method returned the chunk. |

A result can appear in only one candidate list; in that case only that method's
score and rank are present. Results with no matching chunks return
`"results": []`.

## Scoring

Each method retrieves up to `SEARCH_CANDIDATE_POOL` candidates (default 100).
RRF merges them by chunk ID:

```text
score = sum over methods of 1 / (k + rank)
k = SEARCH_RRF_K (default 60)
```

For example, a chunk ranked 1st by both methods scores `1/61 + 1/61`. The score
is useful for ordering, not for absolute relevance thresholds. Ties are broken
by semantic rank, then lexical rank, then chunk ID.

The default `k=60` follows the common RRF recommendation: it dampens the
influence of top ranks so one method cannot dominate solely because its scores
have a different scale.

## Why hybrid retrieval

- **Semantic search** handles paraphrases, synonyms, and conceptual questions.
  BGE-small-en-v1.5 is a small English bi-encoder; it runs comfortably on CPU.
- **Lexical search** handles exact names, product names, numbers, and unusual
  terms that a small embedding model may blur. PostgreSQL uses the `english`
  text-search configuration and `websearch_to_tsquery`, which accepts familiar
  search syntax such as quoted phrases and `OR`.
- **RRF** combines rankings without calibrating cosine similarity against
  `ts_rank_cd`.

The trade-offs are deliberate: a bi-encoder is less precise than a cross-encoder
reranker, and English full-text search does not stem or tokenize other languages
well. See [Architecture](architecture.md#why-this-retrieval-design).

## Examples

Basic search through the UI proxy:

```sh
curl -X POST http://localhost:3000/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"local embedding model","limit":5}'
```

Restrict to one video:

```sh
curl -X POST http://localhost:3000/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"replication","limit":10,"video_id":"VIDEO_UUID"}'
```

Paginate:

```sh
curl -X POST http://localhost:3000/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"replication","limit":10,"offset":10}'
```

Search by direct API:

```sh
curl -X POST http://localhost:8080/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"replication"}'
```

## Indexing requirements

A video is searchable only after its embedding job completes. Check:

```sh
curl http://localhost:3000/api/v1/videos/VIDEO_ID/processing
```

`status` must be `completed`. If the job failed, the response includes
`last_error`. Re-run processing with:

```sh
curl -X POST http://localhost:3000/api/v1/videos/VIDEO_ID/process
```

If the database is small, PostgreSQL may choose a sequential scan instead of the
HNSW index. That is a planner decision, not a correctness problem; HNSW becomes
useful as the chunk table grows.

## Evaluation helper

`api/cmd/evaluate-search` compares vector-only, lexical-only, and hybrid
retrieval against a labeled fixture:

```sh
cd api
DATABASE_URL='postgresql://searcher:searcher@127.0.0.1:5432/searcher' \
  go run ./cmd/evaluate-search
```

It prints average Recall@5, Recall@10, and MRR as JSON. The fixture in
`api/cmd/evaluate-search/queries.json` references chunks from the original
development dataset, so it is only meaningful against that dataset. Replace the
labels with IDs from your own indexed content before drawing conclusions.

## Limits and caveats

- Query embedding uses the same resident model and is serialized with other
  inference in the API process. Search latency therefore depends on CPU and
  concurrent indexing work.
- `SEARCH_TIMEOUT` (default 10s) covers query embedding plus both database
  queries. A timeout returns `SEARCH_FAILED` (HTTP 500).
- The lexical configuration is `english`. Non-English transcript text can still
  match through embeddings, but lexical recall is weaker.
- There is no result highlighting in the API; the frontend highlights query
  terms client-side.
- Uploaded transcripts have no video player. `timestamp_url` is only produced
  when a valid original URL is stored.
- No reranking, query expansion, spell correction, or answer synthesis is
  performed.
