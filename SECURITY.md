# Security Policy

## Supported versions

Sutra is pre-1.0 and does not maintain long-lived release branches. Security
fixes are applied to the `master` branch. Container images published from tags
are snapshots; pull or rebuild from a current commit for fixes.

| Version | Supported |
|---|---|
| `master` (latest) | Yes |
| Older commits/tags | No guaranteed support |

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Use GitHub's private vulnerability reporting for this repository:

<https://github.com/berzz26/searcher/security/advisories/new>

If private reporting is unavailable, contact the maintainer
[@berzz26](https://github.com/berzz26) through GitHub and ask for a private
channel. Include:

- A description of the issue and affected component.
- Reproduction steps or a proof of concept.
- Impact and any known prerequisites.
- Suggested fix, if you have one.
- Whether you intend to disclose publicly and on what timeline.

Do not include live credentials, personal data, or third-party confidential
information in the report.

This is a volunteer project; response times are best effort. We will acknowledge
valid reports, investigate, and coordinate disclosure when practical. We may ask
you to keep the report private until a fix is available.

## Security model and defaults

Sutra is designed as a single-user, self-hosted application.

- **No authentication or authorization.** Anyone who can reach the frontend can
  read, ingest, search, and delete data. There are no user accounts or roles.
- **No built-in TLS.** Terminate TLS at a reverse proxy.
- **No API rate limiting.** Ingestion concurrency is bounded, but there is no
  per-client quota. Put rate limits at the proxy if needed.
- **Database access is trusted.** The API uses a single database role from
  `DATABASE_URL`.
- **Outbound ingestion is restricted to YouTube URLs.** URL validation only
  accepts HTTPS YouTube hosts and known video paths. yt-dlp is invoked without a
  shell.
- **Local inference.** Embeddings run through ONNX Runtime in the API process.
  No transcript text is sent to an external AI API.
- **Artifact storage is optional.** If enabled, credentials are read from the
  environment and the configured bucket is trusted.
- **Containers.** The API runs as UID 10001; services use
  `no-new-privileges`. The default Compose file does not publish the database or
  MinIO and binds the API to loopback.

### Hardening checklist

For any instance reachable by other people:

1. Set strong, unique database and object-store credentials.
2. Put the frontend behind TLS and authentication (OIDC, SSO, basic auth, VPN,
   or an authenticating proxy).
3. Restrict network access to the Docker host and containers.
4. Keep Docker, base images, Go/Node dependencies, yt-dlp, Deno, and ONNX
   Runtime current.
5. Back up the database and test restores.
6. Review logs and monitor for unexpected ingestion or deletion activity.
7. Do not expose port 8080 or the database port.

## Historical credential exposure

Earlier repository history contained deployment credentials in tracked files.
The affected commits are `3041e84` through `9b1c021`; commit `e1551b1` removed
them from the current tree. The repository history was intentionally **not**
rewritten, so the values remain recoverable from old commits.

The exposed material included:

- A PostgreSQL password.
- A MinIO/S3-compatible endpoint, access key, and secret key.

If you cloned, forked, or deployed any commit in that range, or reused its
configuration:

1. Treat the database password and object-store credentials as compromised.
2. Rotate the database password and invalidate old sessions/roles as
   appropriate.
3. Rotate the S3/MinIO access key and secret, and audit bucket access.
4. Check whether the exposed host is still reachable and remove any stale DNS or
   firewall exposure.
5. Review database and object-store logs for unexpected access.
6. Do not rely on deleting `.env` or rewriting the current branch to remediate
   credentials that were already published.

The local untracked `.env` used during development may also contain old values.
It is ignored by Git but should still be rotated and replaced with fresh
credentials.

## Dependency and supply-chain security

- Model, ONNX Runtime, yt-dlp, and Deno are pinned by version and SHA-256.
- The model downloader verifies both checksum and size before use.
- Dependabot is configured for Go, npm, Docker, and GitHub Actions updates.
- CI runs tests, static checks, Compose validation, and Docker builds.
- Release images are built from the repository's workflows.

If you discover a compromised upstream artifact or a checksum mismatch, report
it privately and do not bypass verification.

## Scope

In scope:

- The Go API, frontend, Dockerfiles, Compose files, scripts, and model-init.
- Authentication/authorization gaps, injection, SSRF, path traversal, unsafe
  file handling, secret exposure, denial of service, and supply-chain issues in
  this repository.
- Container configuration and default deployment exposure.

Out of scope:

- Vulnerabilities in YouTube, yt-dlp, ONNX Runtime, PostgreSQL, pgvector, or
  other third-party projects (report them upstream, though we welcome a heads-up
  if Sutra's configuration amplifies the impact).
- Issues that require an already-compromised host or Docker daemon.
- Deliberately exposing an unauthenticated instance to the internet.
- Findings that only apply to unsupported ARM64 or non-Linux deployments.
- Social engineering and physical attacks.

## Disclosure

We prefer coordinated disclosure. Please give us a reasonable opportunity to
investigate and release a fix before public disclosure. Credit can be included
in the advisory or release notes if you want it.
