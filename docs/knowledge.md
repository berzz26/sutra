# Knowledge Log

Historical log of implemented features, tests run, and known limitations. Entries
are newest-first and describe the state at the time they were written; the
current architecture and operator documentation live in the other files in this
directory. Append a new entry after a significant implementation or phase.
See `docs/rules.md`.

## Fix — Full transcript copy/download (2026-11-08)

**Problem**

- Copy and "Download .srt/.vtt" on a video card produced only the first
  1–2 minutes of the conversation. The transcript endpoint is paginated
  (`limit` default 50, `page()` in `api/internal/api/ingestion.go`), and the
  frontend called `GET /api/v1/videos/:id/transcript` without parameters, so
  only the first page's cues were ever loaded — the database was fine
  (verified: cue counts up to 14,774 fully stored).

**Implemented**

- `frontend/src/api/endpoints.ts`: `getTranscript` now accepts `limit` /
  `offset` and builds the query string.
- `frontend/src/pages/SearchPage.tsx`:
  - `fetchAllCues` pages through the transcript endpoint (200 per page)
    until a short page or the transcript's declared `cue_count` is reached.
  - `showTranscript` loads the full transcript with a visible loading state
    before enabling Copy/Download.
  - The viewer renders cues progressively (400 at a time, "Show more") so a
    15k-cue transcript does not freeze the DOM; Copy/Download always encode
    the complete set.
- `frontend/src/styles.css`: styles for the loading row and "show more"
  footer.

**Tested**

- `npm run build` (tsc strict + vite) — clean.
- Live backend pagination check: tail page `?limit=200&offset=14600`
  returns 174 cues starting at seq 14601; a full loop over the WAN Show
  April 3, 2026 video fetched **14,774/14,774 cues across 74 pages**.

**Known limitations**

- Copying a full 15k-cue transcript puts a ~1 MB string on the clipboard;
  browsers accept it but some autofocus contexts can be slow.

## Phase 3 — Search timing metadata (2026-11-02)

**Implemented**

- Search responses now include backend `duration_ms`, measuring query
  embedding plus database retrieval.
- The frontend measures the end-to-end request with `performance.now()` and
  displays both client-resolved time and backend search time near the results,
  including empty-result searches.

**Tested**

- Go tests/vet/build and frontend typecheck/build passed after the change.

## Phase 3 — Timestamped search result URLs (2026-11-02)

**Implemented**

- Search results now include `timestamp_url` whenever a valid original video
  URL exists. The backend derives it from the chunk's `start_ms`, preserves
  existing URL parameters, and adds YouTube-compatible `t` seconds.
- The frontend opens the backend-provided timestamp URL directly instead of
  reconstructing it client-side.

**Tested**

- Added coverage for query-parameter preservation, millisecond-to-second
  conversion, and invalid URL handling.
- Go and frontend checks passed after the change.

## Phase 3 — Hybrid semantic retrieval and search (2026-11-02)

**Implemented**

- Added migration `0005_search_text.sql` with a generated English `tsvector`
  column and GIN index on `transcript_chunks`.
- Added parameterized pgvector cosine candidate retrieval using the existing
  HNSW index, plus PostgreSQL `websearch_to_tsquery('english', ...)` lexical
  retrieval.
- Reused the resident Phase 2 embedder for normalized query vectors and added
  deterministic Reciprocal Rank Fusion by chunk ID. Defaults are 100 candidates
  per method and RRF constant 60.
- Added `POST /api/v1/search` with query, limit, offset, and optional video ID
  filtering, consistent validation/error envelopes, timestamps, video titles,
  original URLs, and documented RRF scores.
- Added a compact frontend search form with loading/error/empty states and
  links to YouTube moments at result timestamps.
- Added `cmd/evaluate-search` and a small labeled fixture comparing vector,
  lexical, and hybrid retrieval on the indexed development dataset.

**Tested**

- `go test ./...`, `go vet ./...`, and `go build ./...` passed.
- Frontend `npm run typecheck` and `npm run build` passed.
- Real server startup applied `0005_search_text.sql` successfully.
- Real `/api/v1/search` queries returned indexed chunks; video filtering,
  invalid queries, invalid IDs, and empty lexical matches were exercised.
- PostgreSQL planning showed the GIN index for lexical search. HNSW was
  confirmed usable with sequential scans disabled; PostgreSQL chose a scan for
  the current 339-row dataset because it is cheaper at that size.
- `go run ./cmd/evaluate-search` reported on three labeled queries: hybrid
  Recall@5 1.0, Recall@10 1.0, MRR 1.0; lexical Recall@5 0.5, Recall@10 0.667,
  MRR 0.5; vector Recall@5 1.0, Recall@10 1.0, MRR 0.833.
- Three live API timings for a 10-result query were 11.6–16.1 ms on the
  current local database/model process; this is not a production benchmark.

**Known limitations**

- Evaluation labels are tied to the current development transcript set and
  must be replaced with deployment-specific human labels for meaningful quality
  evaluation.
- Uploaded transcripts have no video player; timestamp links are actionable
  only when a result has an original YouTube URL.
- Reranking and answer generation remain intentionally out of scope.

## Phase 2 — Transcript processing and semantic indexing (2026-11-02)

**Implemented**

- Replaced sentence-splitting with deterministic, cue-preserving chunking using
  the configured 180-word target, 80-word minimum, 250-word maximum, and
  complete-cue overlap up to 30 words. Empty cues are ignored while raw cues
  remain authoritative.
- Completed resident BGE-small-en-v1.5 ONNX inference with one startup-loaded
  tokenizer/session, batched dynamic tensors for `input_ids`,
  `attention_mask`, and `token_type_ids`, 512-token truncation, mask-aware mean
  pooling, L2 normalization, finite-value checks, and 384-dimension validation.
- Added idempotent transactional chunk upserts and vector persistence. Retries
  use deterministic IDs and the `(video_id, seq)` uniqueness constraint instead
  of deleting/recreating the entire derived set.
- Added migration `0004_processing_pipeline.sql`: retry scheduling, one active
  embedding job per video, and a partial HNSW cosine index over non-null
  embeddings. Migration errors involving pgvector now explain the required
  extension instead of silently falling back.
- Added PostgreSQL-backed workers with `FOR UPDATE SKIP LOCKED`, bounded
  concurrency, attempts, exponential retry backoff, structured lifecycle logs,
  graceful shutdown, and `go run ./cmd/backfill` for resumable existing-data
  indexing.
- Newly successful ingestion now enqueues the same embedding pipeline used by
  backfill and manual processing.

**Tested**

- Added unit tests for normal, short, long, unusually long/overlapping, empty,
  timestamp-preserving, and deterministic chunks.
- Added embedding pooling tests for attention masks, normalization, non-finite
  values, and all-padding rejection.
- `go test ./...` passed.
- Production server startup against PostgreSQL initialized the actual model,
  tokenizer, and ONNX session and applied migration `0004`.
- Backfill processed three existing transcripts into 339 normalized 384-value
  pgvector embeddings. A real multipart upload then created a background job
  and produced one embedded chunk; the smoke record was deleted afterward.
- Verified repeated processing returns the completed state without duplicating
  chunks, and `go run ./cmd/backfill` is safe for retrying a failed transcript.

**Known limitations**

- PostgreSQL integration tests are not yet isolated in an automated test
  harness; database behavior was verified against the configured PostgreSQL
  instance during this implementation pass.
- Semantic retrieval and reranking query APIs remain future phases.

## Phase 1 — Subtitle discovery API (2026-11-02)

**Implemented**

- Added `GET /api/v1/videos/subtitles?url=...` using yt-dlp metadata only;
  no subtitle, video, or audio is downloaded during discovery.
- The response lists compatible manual tracks only with exact yt-dlp language
  keys, source type, display name when available, and formats; automatic tracks
  are intentionally excluded from discovery/UI.
- Added `subtitle_key` and `subtitle_source` request fields to YouTube
  ingestion so the selected track is fetched exactly, including `en` versus
  `en-orig`.
- Updated the frontend to discover tracks, let the user select one, and fetch
  that exact transcript.

**Tested**

- Added unit tests for discovery options and exact manual/automatic selection.
- `go test ./...` passed.
- `npm run build` passed from `frontend/`.

**Known limitations**

- Discovery currently returns tracks that expose VTT or SRT support, while
  reporting all advertised formats for those tracks.

## Phase 1 — Subtitle language selection correction (2026-11-02)

**Implemented**

- Root cause addressed: yt-dlp's translated automatic `en` track can be rate
  limited while the original-language `en-orig` track succeeds.
- Subtitle selection now preserves exact yt-dlp keys and prioritizes manual
  tracks, original automatic tracks, regular automatic tracks, and deterministic
  regional variants without crossing languages.
- Added structured debug logging for requested, available, selected language,
  and source type.
- Added `YTDLP_RATE_LIMITED` classification for HTTP 429/too-many-request
  failures and mapped it to HTTP 429.

**Tested**

- Added table-driven selection tests for manual precedence, `en-orig`, regular
  fallback, regional exact matches, deterministic variants, unavailable
  languages, and generated yt-dlp arguments.
- Real YouTube verification was attempted against
  `https://www.youtube.com/watch?v=bMhB47Nb2MA`; metadata exposed `en-orig`,
  the direct subtitle download succeeded, and full API ingestion with requested
  `en` completed as `language: en-orig`, `auto_generated`, 442 cues. The test
  video record was deleted afterward.

**Known limitations**

- No automatic retries, proxy rotation, cookies, or authentication were added
  for rate limiting.

## Phase 1 — Transcript ingestion (2026-11-02)

**Implemented**

- Added migration `0002_transcript_ingestion.sql` with video metadata,
  lifecycle statuses, and one transcript summary per video. Existing raw cues
  remain in `transcript_cues`; `transcript_chunks` is untouched.
- Added pure VTT/SRT parsing and normalization with millisecond timestamps,
  multiline cues, metadata blocks, entities, formatting cleanup, validation,
  and sequence preservation.
- Added strict HTTPS YouTube URL parsing and canonical video-ID deduplication.
- Added yt-dlp metadata/subtitle extraction with manual-before-automatic
  language selection, timeouts, no-playlist/no-media flags, temporary cleanup,
  and classified failures.
- Added synchronous upload and YouTube ingestion services, transactional cue
  persistence, idempotent upload hashing, optional MinIO artifact storage,
  paginated cue retrieval, deletion, and Fiber routes.
- Added the minimum React ingestion/library UI for URL submission, VTT/SRT
  upload, cue inspection, and deletion.

**Tested**

- `go test ./...` — passed, including URL and parser unit tests.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `gofmt -l api` — passed with no output.
- `npm run build` — passed.
- Queried the live PostgreSQL 16 instance before migration, then started the
  server and verified `0002_transcript_ingestion.sql` was applied.
- Manual API smoke test: uploaded a VTT through multipart HTTP, listed it,
  retrieved/deleted it, and checked invalid URL and playlist errors.
- Real yt-dlp attempt: `yt-dlp --dump-single-json --skip-download
  --no-warnings --no-playlist https://www.youtube.com/watch?v=BaW_jenozKc`;
  yt-dlp ran successfully but the selected public test video was unavailable,
  so no real subtitle extraction was completed.

**Known limitations**

- Real YouTube ingestion still requires a network-accessible, installed
  `yt-dlp` and was not run during this implementation pass.
- The frontend is intentionally a compact ingestion/library surface; semantic
  search, chunking, and embeddings remain later phases.
- Artifact deletion after a successful database deletion can leave an orphan
  object if the object store is unavailable; the API reports that cleanup
  failure and never leaves a database pointer to a missing object.

## Phase 0 — Repository foundation (2026-11-02)

**Implemented**

- Go backend scaffold:
  - Entry point `backend/cmd/server/main.go`: config load → structured logging
    (`log/slog`) → PostgreSQL pool → migrations → Fiber HTTP server.
  - Fiber app with recover + request-id middleware and a central error
    handler (`backend/internal/server`).
  - Health/readiness endpoints: `GET /healthz` (liveness) and `GET /readyz`
    (pings the database with a 2s timeout), plus `GET /api/v1/health`.
  - Config loading from environment variables (`backend/internal/config`),
    including optional artifact-store (MinIO) settings behind
    `ARTIFACT_STORE_ENABLED`.
  - pgx connection pool management (`backend/internal/persistence/db.go`).
  - File-based, embedded, exactly-once migration runner
    (`backend/internal/persistence/migrations.go`,
    `backend/cmd/server/migrations/0001_init.sql`). Applied versions are
    tracked in `schema_migrations`; each migration runs in one transaction.
- Database schema: `videos`, `transcript_cues` (raw cues, immutable),
  `transcript_chunks` (indexed search chunks, embeddings added later),
  `jobs` (table-backed queue for in-process workers). UUID primary keys,
  status/sequence checks, unique constraints and indexes.
- Pipeline scaffolds for ingestion, processing, embedding, retrieval,
  reranking, and jobs — packages exist and compile but contain no
  functionality yet, as this phase defines structure only.
- React + TypeScript frontend scaffold:
  - Vite build, strict TypeScript, React Router routing
    (`/`, `/status`, 404 route).
  - API client abstraction (`src/api/client.ts`) with typed `ApiError` and
    centralized error handling.
  - App shell (header/nav layout), global error boundary, status page that
    calls the backend health endpoint.
  - Env config: `VITE_API_BASE_URL` (client) and `VITE_API_PROXY_TARGET`
    (dev/preview proxy).
- Docker Compose: PostgreSQL 16 (health-checked), backend, frontend; plus
  Dockerfiles for both.
- Docs: `README.md`, `docs/architecture.md`, `docs/rules.md` (this workflow),
  `.env.example`.

**Tested**

- `go build ./...`, `go vet ./...`, `gofmt -l` — clean.
- Server started against the live PostgreSQL instance; migration
  `0001_init.sql` applied successfully and is idempotent on restart.
- `GET /healthz`, `GET /readyz`, `GET /api/v1/health` all return 200 with
  correct payloads; `/readyz` fails with 503 when the database is
  unreachable. Unknown routes return a JSON error envelope
  (`{"error":"404 Not Found"}`, status 404).
- Graceful shutdown: SIGTERM → "shutting down gracefully" → "drained all
  connections" → exit code 0.
- Frontend: `npm run build` (tsc + vite) clean; Vite dev server proxies
  `/api/v1/health` to the running backend and returns 200; SPA fallback
  routes (e.g. `/status`) serve the app.
- `docker compose config` validated.

**Known limitations**

- Port 8080 is occupied by an unrelated service on the development machine;
  local runs use `SERVER_PORT=18080` (via `.env` / `frontend/.env`).
- No pgvector extension yet; `transcript_chunks` has no embedding column.
  Embedding storage is added together with the embedding feature in a later
  phase.
- Artifact store (MinIO) is configured but not wired to any code path.
- Frontend production serving uses `vite preview`; a reverse proxy in front
  of static assets is expected for hardened deployments.

## Frontend UI/UX revamp (2026-11-02)

**Implemented**

- Reworked the React application shell with responsive navigation, persistent
  light/dark theme switching, an application footer, Lucide icons, and Sonner
  notifications.
- Redesigned the search experience with a prominent natural-language search
  surface, suggestion states, result metadata, relevance/timing indicators,
  timestamp links, query-term highlighting, staggered result entry, and
  reduced-motion support.
- Reworked YouTube ingestion and transcript upload into dedicated cards with
  subtitle discovery, drag-and-drop upload, loading states, and success/error
  notifications.
- Reworked the video library and transcript viewer with source/status badges,
  cue metadata, skeleton loading, responsive layouts, and an in-app destructive
  action confirmation dialog.
- Added reusable shadcn-style UI primitives in `frontend/src/components/ui.tsx`
  for buttons, inputs, cards, badges, separators, skeletons, spinners, and
  alert dialogs. Existing API routes and data contracts were preserved.

**Tested**

- `npm run typecheck` — passed.
- `npm run build` — passed.
- `git diff --check` — passed.

**Known limitations**

- No portfolio implementation was present in the workspace, so the visual
  system was derived from the requested editorial developer-product direction.
- The existing API does not expose richer thumbnail or indexing-progress data;
  library cards therefore only show metadata already available from the API.

## Discovery-style search results (2026-11-02)

- Replaced the flat ranked search result list with a responsive discovery grid:
  three columns on large screens, two on medium screens, and one on mobile.
- Search results are grouped by `video_id`; each video appears once using its
  strongest returned moment, while additional moments remain available in a
  dialog without changing the underlying result set.
- Cards use the existing video library `thumbnail_url` metadata, with a local
  fallback surface when no thumbnail exists. Raw rank numbers and relevance
  percentages are no longer shown to users.
- Added loading skeleton cards, timestamp actions, transcript highlighting,
  and reduced-motion-compatible entrance/hover treatments.
- No backend or search API changes were made.

## Horizontal search result cards (2026-11-02)

- Restored one card per retrieved matching chunk instead of grouped discovery
  cards. Each card is larger and split horizontally: thumbnail, timestamp, and
  source metadata on the left; title, transcript excerpt, and open-moment action
  on the right.
- Removed the grouping dialog and in-place excerpt expansion from search
  results while keeping the original result set and timestamp behavior.

- Removed the translated-subtitles dropdown from the frontend ingestion form;
  the backend discovery and exact-track support remain unchanged.

## Progressive flux indexing loader (2026-11-02)

- Replaced the plain indexing progress bar with a controlled Framer Motion
  `ProgressiveFluxLoader` component.
- The loader follows backend progress values and maps them to transcript fetch,
  queueing, chunking, embedding, saving, and ready phases.
- It includes animated flux/sheen treatment, accessible progress semantics,
  reduced-motion support, and a failure color variant.

## Transcript export controls (2026-11-03)

- Added Copy and Download controls to the expanded transcript viewer.
- Copy places the complete cue transcript on the clipboard.
- Download generates browser-side SRT or VTT output using the transcript's
  stored original format, preserving cue order, timestamps, and text.
- Export actions do not require backend changes or additional API calls.
- Clipboard copy also falls back to a temporary textarea plus
  `document.execCommand('copy')` when the modern Clipboard API is unavailable
  or blocked on local/non-secure deployments.

**Tested**

- `npm run typecheck` — passed.
- `npm run build` — passed.
- `git diff --check` — passed after removing generated TypeScript build info.

## Product rename to Sutra (2026-11-02)

- Updated the product-facing name to `sutra` in the frontend title, brand,
  theme storage key, status copy, package name, backend application name,
  binary name, Docker entrypoint, temporary ingestion directory prefix, and
  README heading.
- Kept existing PostgreSQL credentials and Go module import path unchanged to
  avoid breaking existing deployments or downstream imports.
- Verified the frontend still passes `npm run typecheck` and `npm run build`.

## Unified ingestion flow and pipeline progress (2026-11-02)

**Implemented**

- Consolidated YouTube ingestion and local VTT/SRT upload into one ingestion
  card with accessible source tabs.
- YouTube discovery now returns title, thumbnail URL, duration, channel, video
  ID, and available subtitle tracks. English is selected automatically when an
  English track is available; users can choose another track before indexing.
- The explicit YouTube flow is now `Fetch video` (metadata/subtitle discovery)
  followed by `Upload & index` (subtitle ingestion plus asynchronous indexing).
- Added persisted processing progress fields through migration
  `0006_processing_progress.sql`: stage, percentage, cue count, and embedding
  count. Processing updates these at chunking, embedding, and save stages.
- The frontend polls the existing processing endpoint after ingestion and
  displays an end-to-end progress bar until the transcript is searchable.

**Tested**

- `go test ./...`, `go vet ./...`, and `go build ./...` — passed.
- `npm run typecheck` and `npm run build` — passed.
- `git diff --check` — passed.

**Known limitation**

- Subtitle fetching remains part of the synchronous ingestion HTTP request;
  the progress bar begins immediately in the UI and becomes backend-backed as
  soon as the video row and processing job exist.

## Subtitle discovery correction (2026-11-02)

- Discovery now returns all compatible manual and automatic YouTube caption
  tracks instead of manual tracks only.
- The UI uses `English` as the default subtitle option and keeps the upload
  action enabled even when no track was selected, allowing the backend's
  default `en` language selection to run.
- The frontend language value now defaults and resets to `en` for both source
  tabs.
- Updated subtitle option coverage in ingestion tests.

## Original versus translated subtitle selection (2026-11-02)

- `subtitleEntry` now retains the URL returned by yt-dlp. Automatic tracks are
  classified from the URL's `tlang` metadata, so translated tracks are not
  inferred from or assigned synthetic language IDs.
- Discovery returns manual tracks plus original automatic tracks in the default
  list and exposes translated automatic tracks in a separate API collection.
- The UI keeps translated tracks behind a collapsed opt-in section. Selecting
  one still passes its exact language key and source to the existing download
  operation.
- Default selection and fallback logic no longer chooses translated automatic
  tracks. General yt-dlp HTTP 429 classification remains unchanged.
- Added tests for metadata-based classification, translated-track exclusion,
  original fallback, and exact download arguments.

## Processing API response simplification (2026-11-02)

- Processing persistence still retains model and processing versions for
  internal idempotency and compatibility checks.
- The HTTP processing endpoints now return only progress-facing fields:
  status, stage, percentage, cue/embedding/chunk counts, video ID, and an
  optional failure message. Internal model/version metadata is no longer
  exposed to clients.
- Docker Compose has not been end-to-end executed yet (builds validated via
  Docker config; local run tested natively against the live DB).
