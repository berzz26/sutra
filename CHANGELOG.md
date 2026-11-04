# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Sutra is pre-1.0, so
minor versions may include breaking changes.

## [Unreleased]

## [0.1.0] - 2026-11-03

Initial open-source release of Sutra.

### Added

- One-command Docker Compose stack: PostgreSQL 16 + pgvector, checksum-verified
  model initializer, Go API with in-process workers, and nginx-served React
  frontend.
- YouTube subtitle ingestion with exact-track discovery, manual/original
  automatic track selection, canonical URL validation, and structured errors.
- VTT/SRT transcript uploads with content sniffing, size limits, and SHA-256
  deduplication.
- Local BGE-small-en-v1.5 ONNX embeddings with batch inference, mask-aware mean
  pooling, and 384-dimensional normalization.
- Deterministic cue-preserving chunking with overlap and a PostgreSQL-backed
  embedding job queue.
- Hybrid search using pgvector cosine candidates, PostgreSQL English full-text
  candidates, and Reciprocal Rank Fusion, with timestamped YouTube links.
- Web UI for ingestion, progress, library management, transcript copy/download,
  and search.
- Optional S3-compatible artifact storage for original subtitle files.
- Embedded, forward-only SQL migrations applied at startup.
- Local development workflow with Make targets, host model/runtime fetch
  scripts, dev Compose overrides, and a real HTTP smoke test.
- Operator documentation for installation, configuration, architecture,
  database, ingestion, search, models, API, backup/restore, and troubleshooting.
- Contributor and project files: MIT license, contributing guide, code of
  conduct, security policy, support guide, issue/PR templates, Dependabot, and
  CI/release workflows.
- Third-party notices and license texts for pinned artifacts.

### Changed

- Replaced the previous development Dockerfiles with multi-stage, non-root,
  checksum-verified production images.
- Frontend is served by unprivileged nginx on internal port 8080 and proxies
  `/api` to the API, removing the need for CORS or a build-time backend URL.
- Compose now uses named volumes, health checks, startup ordering, loopback-only
  API publishing, and no host database port.
- Database and MinIO ports are only published through the optional dev override,
  bound to loopback.
- Model weights and ONNX Runtime are no longer expected in the repository; they
  are provisioned into a named volume or fetched for host development.
- MinIO is profile-only because artifact storage is optional and upstream
  community image availability is uncertain.
- Consolidated the environment reference in `.env.example` and documented every
  runtime setting.
- Rewrote the README as a product and operator landing page.

### Fixed

- Removed generated TypeScript build-info files from version control.
- Cleaned up Docker build context and added repository-wide ignore rules for
  secrets, model weights, runtime archives, and build output.
- Formatted Go sources and added CI checks for formatting, vet, tests, builds,
  frontend typecheck/build, Compose validation, and Docker builds.

### Security

- Added pinned SHA-256 verification for the model, tokenizer, ONNX Runtime,
  yt-dlp, and Deno.
- Added a private vulnerability-reporting policy and deployment hardening
  guidance.
- Documented historical credential exposure: commits `3041e84` through
  `9b1c021` contained database and object-store credentials that were removed
  in `e1551b1`. History was not rewritten. Deployments based on those commits
  must rotate the affected credentials. See [SECURITY.md](SECURITY.md).

### Known limitations

- No authentication, authorization, or built-in TLS.
- Linux amd64 only for the provided images.
- English-centric embedding model and lexical search configuration.
- No reranking, summarization, or answer generation.
- Single-node in-process workers; no distributed queue.
- YouTube extraction depends on yt-dlp and YouTube's current behavior.
- The optional bundled MinIO profile is not verified end to end.

[Unreleased]: https://github.com/berzz26/searcher/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/berzz26/searcher/releases/tag/v0.1.0
