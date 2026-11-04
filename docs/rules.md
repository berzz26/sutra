# Repository engineering rules

These are the architectural and process rules for changes to Sutra. If a change
must violate one, explain the reason and trade-off in the pull request and, when
appropriate, in [architecture.md](architecture.md).

See [CONTRIBUTING.md](../CONTRIBUTING.md) for setup, tests, and the review
process.

## Architecture constraints

1. **One API binary, one database.** The Go API serves HTTP and runs its
   PostgreSQL-backed workers. Do not add Redis, Kafka, RabbitMQ, a separate
   worker service, or a microservice split.
2. **No external AI APIs.** Embeddings run locally through ONNX Runtime. Do not
   send transcript text or queries to a hosted model.
3. **Raw cues and derived chunks are separate.** `transcript_cues` is the
   immutable source of truth; `transcript_chunks` is derived, versioned search
   data. Do not collapse them.
4. **Model weights and native runtimes are not committed.** Provision them
   through `model-init`, pinned URLs, checksums, and ignored local directories.
5. **Single-node coordination.** Jobs use PostgreSQL row locking and in-process
   workers. Do not imply distributed-worker guarantees.
6. **No speculative abstractions.** Add an interface only when a second real
   implementation needs it. Follow Fiber for HTTP and pgx for SQL.
7. **Privacy and security by default.** No hidden telemetry, no transcript
   upload to third parties, safe container defaults, and no secrets in the
   repository.

## Configuration and dependencies

1. Every runtime setting is an environment variable, documented in
   `.env.example`, `docs/configuration.md`, and the config loader together.
2. Defaults must be safe for local use and clearly marked when they are not
   production credentials.
3. Pin third-party binaries and model artifacts by version and SHA-256. Update
   `THIRD_PARTY_NOTICES.md` and `docs/models.md` when changing them.
4. New dependencies need a clear reason, a compatible license, and a maintained
   upstream. Avoid adding a second framework for an existing concern.
5. Do not commit generated output (`frontend/dist`, `*.tsbuildinfo`, binaries),
   model weights, or runtime archives.

## Code style

1. No excessive commenting. Comments explain constraints, trade-offs, and
   external behavior, not obvious mechanics. Never leave commented-out code or
   placeholder TODO noise.
2. Go follows `gofmt` and `go vet`; TypeScript is strict and must pass
   `npm run typecheck`.
3. No placeholder business logic that pretends to work. If something is not
   implemented, say so in the code, docs, and UI rather than faking it.
4. SQL migrations are numbered `NNNN_description.sql`, forward-only, and
   transactional. Include a reindex note when changing embeddings or chunking.
5. Shell scripts in `scripts/` and `deploy/` are POSIX `sh`.

## Testing and documentation

1. Add or update tests for behavior changes. Run `make test` and `make lint`.
2. Changes affecting Docker, dependencies, ingestion, indexing, or search must
   validate the relevant Docker build and, where possible, `make smoke`.
3. Update documentation when behavior, configuration, APIs, or operations
   change. Do not claim a check passed unless it was actually run.
4. Append significant implementation notes to [knowledge.md](knowledge.md)
   rather than rewriting its history.

## Security

1. Never commit `.env`, credentials, tokens, private keys, or personal data.
2. Treat uploaded transcripts and fetched subtitles as untrusted input: validate
   sizes, formats, and paths.
3. Keep outbound extraction constrained to validated YouTube URLs and invoke
   subprocesses without a shell.
4. Report vulnerabilities privately per [SECURITY.md](../SECURITY.md).
5. Consider the deployment model: Sutra has no built-in authentication or TLS,
   so documentation must not imply that a shared instance is safe by default.
