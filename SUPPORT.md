# Support

Sutra is a volunteer, self-hosted project. There is no commercial support and no
service-level agreement.

## Before asking for help

1. Read the [README](README.md) and the relevant guide:
   - [Installation](docs/installation.md)
   - [Configuration](docs/configuration.md)
   - [Troubleshooting](docs/troubleshooting.md)
   - [API reference](docs/api.md)
2. Check existing GitHub issues and pull requests.
3. Reproduce with the latest `master` if possible.
4. Collect logs and versions.

Useful diagnostics:

```sh
docker compose ps -a
docker compose logs --no-color --tail=200 api model-init db frontend
docker version
docker compose version
uname -a
```

Redact credentials, hostnames, tokens, and personal data before posting.

## Getting help

- **Bug reports and feature requests:** open an issue using the repository's
  issue templates:
  <https://github.com/berzz26/searcher/issues>
- **Security vulnerabilities:** follow [SECURITY.md](SECURITY.md) and report
  privately. Do not open a public issue.
- **Contributing:** see [CONTRIBUTING.md](CONTRIBUTING.md).
- **Code of conduct concerns:** see [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

There are no guaranteed response times. Well-scoped reports with reproduction
steps, versions, and logs are much easier to help with.

## What is not supported

- ARM64/Apple Silicon or Windows container deployments with the provided
  images.
- Hosted multi-tenant operation, authentication, or billing.
- Recovering data deleted by `docker compose down -v` without a backup.
- Bypassing YouTube login, rate limits, or access controls.
- Custom model integrations that change embedding dimensions without a
  migration and reindex.

## Reporting a bug

Include:

- What you expected and what happened.
- Exact steps to reproduce.
- `docker compose ps -a` and relevant log excerpts.
- Host OS, architecture, Docker/Compose versions.
- Whether you used the default Compose stack or host development.
- Whether the issue affects uploads, YouTube ingestion, indexing, search, or the
  frontend.
- Any recent configuration changes (redacted).

Please do not attach a full database dump or `.env` file. A minimal transcript
fixture or a synthetic reproduction is preferred.
