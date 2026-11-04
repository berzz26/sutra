## Summary

<!-- What problem does this change solve, and how? -->

## Related issue

<!-- Link an issue, e.g. "Closes #123". Use "N/A" if there is none. -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Documentation
- [ ] Refactor or maintenance
- [ ] Dependency update
- [ ] CI / build / release

## How was this tested?

<!-- List exact commands and results. Do not claim a check passed unless it ran. -->

```sh
make fmt
make lint
make test
```

- [ ] Backend tests
- [ ] Frontend typecheck
- [ ] Frontend production build
- [ ] Compose validation
- [ ] Docker image build
- [ ] End-to-end smoke test (`make smoke`)
- [ ] Manual UI check

## Checklist

- [ ] The change is focused and does not include unrelated edits.
- [ ] Tests were added or updated for behavior changes.
- [ ] Documentation and `.env.example` were updated if configuration or behavior changed.
- [ ] No `.env`, credentials, tokens, model weights, runtime archives, or generated build output are included.
- [ ] The Go module path and the single-binary/single-database architecture were preserved.
- [ ] Raw cues and derived chunks remain separate.
- [ ] New or changed third-party artifacts have pinned versions, checksums, licenses, and notices.
- [ ] Database changes are forward-only migrations and include an upgrade/reindex note if needed.
- [ ] Security implications were considered (auth, SSRF, uploads, logs, secrets, deployment exposure).

## Screenshots or logs

<!-- For UI changes, include before/after screenshots. Redact secrets. -->

## Notes for reviewers

<!-- Trade-offs, follow-ups, or parts that need special attention. -->
