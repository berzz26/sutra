# Contributing to Sutra

Thanks for helping improve Sutra. This project is a self-hosted semantic video
search engine; changes should keep the single-binary/single-database design and
the local-inference privacy model intact.

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).

## Before you start

- Read the [Code of Conduct](CODE_OF_CONDUCT.md).
- Report security issues privately as described in [SECURITY.md](SECURITY.md),
  not in a public issue.
- Search existing issues before opening a new one.
- For significant changes, open an issue first so the approach can be discussed.

## Development setup

Requirements:

- Go 1.26+
- Node.js 22.12+
- Docker with Compose v2.20+

```sh
git clone https://github.com/berzz26/searcher.git
cd searcher
make dev-deps
make dev
```

`make dev` starts PostgreSQL in Docker and runs the API and Vite on the host.
See [Development](docs/development.md) for details, including separate-terminal
commands and Docker-only development.

## What to work on

Useful contributions include:

- Search quality evaluation and retrieval improvements.
- Better transcript parsing and subtitle edge cases.
- Frontend accessibility, usability, and responsive behavior.
- Documentation, examples, and troubleshooting.
- Dependency updates and reproducibility improvements.
- Tests for existing behavior.

Please avoid:

- Replacing the architecture with microservices, an external queue, or a
  different database.
- Adding external AI APIs or sending transcript data to third parties.
- Adding speculative abstractions or placeholder features that appear to work.
- Committing model weights, ONNX Runtime archives, generated frontend output, or
  secrets.
- Renaming the Go module path `github.com/berzz/searcher/backend` without a
  coordinated migration plan.

The full engineering rules are in [docs/rules.md](docs/rules.md).

## Making a change

1. Fork the repository and create a topic branch from `master`.
2. Keep changes focused. Separate refactors from behavior changes when possible.
3. Add or update tests for behavior changes.
4. Update documentation and `.env.example` when configuration or operations
   change.
5. Run the checks below.
6. Open a pull request using the template.

Suggested branch names: `fix/upload-dedupe`, `docs/backup-guide`,
`feat/subtitle-filter`.

## Required checks

Run these before opening a pull request:

```sh
make fmt
make lint
make test
```

For frontend changes, also run a production build:

```sh
cd frontend
npm run build
```

For Docker or dependency changes, build the images and validate Compose:

```sh
make config
docker compose build
```

For changes that affect ingestion, indexing, or search, run the end-to-end smoke
test against a running stack:

```sh
docker compose up --build -d
make smoke
```

If a check cannot run in your environment, say so in the pull request and
explain what you verified instead. Do not claim a check passed if it was not
run.

## Tests

- Go tests live beside the package under test (`*_test.go`).
- Run `go test ./...` from `api/`.
- The frontend currently has no test runner; it is checked with `tsc` and a Vite
  production build.
- `scripts/smoke-test.sh` exercises the real HTTP API and requires a running
  stack. It is not a unit test.

New behavior should have a regression test when practical. For retrieval,
include a small, clearly labeled fixture rather than hard-coding a private
dataset.

## Coding style

- Go: `gofmt`, `go vet`, standard library conventions, explicit error wrapping.
- TypeScript/React: strict mode, existing component and styling patterns,
  `npm run typecheck`.
- SQL: migrations are forward-only and numbered
  `NNNN_description.sql`; keep them transactional and idempotent where possible.
- Shell: POSIX `sh` for scripts in `scripts/` and `deploy/`; pass `sh -n` and
  avoid Bash-only syntax.
- Comments explain constraints and trade-offs, not obvious mechanics. Do not
  leave commented-out code or placeholder TODOs.

## Configuration and secrets

- Never commit real credentials, `.env` files, tokens, or private keys.
- Add new configuration to `.env.example`, [docs/configuration.md](docs/configuration.md),
  and the Go config loader together.
- Defaults must be safe for local use and clearly documented as non-production.
- If you find a secret in the repository, report it privately rather than
  opening a public issue with the value. See [SECURITY.md](SECURITY.md).

## Models and third-party artifacts

- Do not commit model weights or native runtime archives.
- Pinned downloads must include an upstream URL, version, SHA-256 checksum, and
  license/notice update.
- Update [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and
  [docs/models.md](docs/models.md) when changing a pinned artifact.
- Changing model dimensions or tokenizer inputs is a breaking index change;
  describe the migration and reindex plan in the pull request.

## Documentation

Documentation lives in `docs/` and is linked from the README. Keep it accurate
and test commands before adding them. `docs/knowledge.md` is a historical
implementation log; append entries for significant changes rather than rewriting
history.

## Commit messages

Use a clear imperative subject, optionally with a conventional prefix:

```text
fix: reject mismatched VTT/SRT uploads
docs: document offline model preload
feat: add processing progress endpoint
```

Explain the reason and trade-offs in the body when they are not obvious. Keep
unrelated changes out of the same commit when possible.

## Pull request review

A pull request is ready when:

- The description explains the problem and approach.
- Tests and documentation are updated.
- Required checks pass in CI.
- No secrets, model weights, or generated artifacts are included.
- The change respects the architecture and privacy constraints.
- Review comments are addressed or discussed.

Maintainers may ask for a smaller change, additional tests, or a design
discussion before merging. There is no guaranteed response time; this is a
volunteer project.

## Release process

Releases are cut by maintainers from tags of the form `vX.Y.Z`. The release
workflow publishes Linux amd64 images to GitHub Container Registry and creates a
GitHub release. See `.github/workflows/release.yml`.
