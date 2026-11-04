# Third-party notices

Sutra is distributed under the [MIT License](LICENSE). It depends on third-party
software and downloads one machine-learning model. This file summarizes the
direct components and pinned artifacts. Full license texts for some components
are in [`third_party/licenses/`](third_party/licenses/).

This list is provided for attribution and is not a legal opinion. Transitive
dependencies are resolved by `api/go.mod` / `api/go.sum` and
`frontend/package-lock.json`; their license metadata is available from the
upstream modules and packages. Base container images also include operating
system packages under their own licenses.

## Downloaded at runtime or build time

| Component | Version | License | Source |
|---|---|---|---|
| BAAI/bge-small-en-v1.5 model and tokenizer | revision `5c38ec7c405ec4b44b94cc5a9bb96e735b38267a` | MIT (per model card) | <https://huggingface.co/BAAI/bge-small-en-v1.5> |
| ONNX Runtime (Linux x64 CPU) | `1.31.1` | MIT | <https://github.com/microsoft/onnxruntime> |
| yt-dlp (`yt-dlp_linux`, bundles yt-dlp-ejs) | `2026.10.26` | Unlicense | <https://github.com/yt-dlp/yt-dlp> |
| Deno | `2.9.8` | MIT | <https://github.com/denoland/deno> |

ONNX Runtime's license and its upstream third-party notices are copied
verbatim:

- [`third_party/licenses/onnxruntime-LICENSE`](third_party/licenses/onnxruntime-LICENSE)
- [`third_party/licenses/onnxruntime-ThirdPartyNotices.txt`](third_party/licenses/onnxruntime-ThirdPartyNotices.txt)

Sutra does not redistribute the model weights. `model-init` downloads them from
the pinned Hugging Face revision and verifies their checksums. If you mirror or
redistribute the model, retain the upstream MIT license and attribution.

## Runtime services and images

| Component | Version | License | Source |
|---|---|---|---|
| PostgreSQL | 16 (Debian bookworm image) | PostgreSQL License | <https://www.postgresql.org/about/licence/> |
| pgvector | 0.8.7 (via `pgvector/pgvector:0.8.7-pg16-bookworm`) | PostgreSQL License | <https://github.com/pgvector/pgvector> |
| nginx | 1.27 (via `nginxinc/nginx-unprivileged:1.27-alpine`) | BSD-2-Clause | <https://nginx.org/> |
| Alpine Linux | 3.20 (model-init image) | Various (mostly MIT/BSD/Apache-2.0) | <https://alpinelinux.org/> |
| Debian | bookworm-slim (API runtime) | Various free-software licenses | <https://www.debian.org/legal/licenses/> |
| Node.js | 22 (frontend build image) | MIT | <https://nodejs.org/> |
| Go | 1.26 (API build image) | BSD-3-Clause | <https://go.dev/> |

The optional `minio` profile uses the image configured by `MINIO_IMAGE`
(default `quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z`). MinIO Community
Edition licensing and image availability are governed by upstream; review them
before deploying the optional profile. Artifact storage can point at any
S3-compatible provider instead.

## Go dependencies (direct)

| Module | License |
|---|---|
| `github.com/gofiber/fiber/v2` | MIT |
| `github.com/google/uuid` | BSD-3-Clause |
| `github.com/jackc/pgx/v5` | MIT |
| `github.com/minio/minio-go/v7` | Apache-2.0 |
| `github.com/sugarme/tokenizer` | MIT |
| `github.com/yalue/onnxruntime_go` | MIT |
| `golang.org/x/*` (indirect) | BSD-3-Clause |

The Fiber, pgx, minio-go, sugarme/tokenizer, and onnxruntime_go repositories
contain their full license texts. `go.sum` records exact module versions.

## Frontend dependencies (direct)

| Package | License |
|---|---|
| `react`, `react-dom` | MIT |
| `react-router-dom` | MIT |
| `framer-motion` | MIT |
| `lucide-react` | ISC |
| `sonner` | MIT |
| `typescript` | Apache-2.0 |
| `vite` | MIT |
| `@vitejs/plugin-react` | MIT |
| `@types/node`, `@types/react`, `@types/react-dom` | MIT |

Exact versions are locked in `frontend/package-lock.json`.

## Base-image packages installed by the API Dockerfile

The API runtime installs `ca-certificates`, `curl`, `libstdc++6`, `tar`, and
`unzip` from Debian. The model initializer installs `ca-certificates`, `curl`,
and `jq` from Alpine. These packages retain their upstream licenses. `curl` uses
the curl license; `jq` is MIT; `libstdc++6` is GPL-3.0-or-later with the GCC
runtime library exception. Consult the Debian/Alpine package metadata for the
complete set of notices.

## Model-init manifest

The pinned model source, revision, checksums, and declared license are in
[`deploy/model-init/model-manifest.json`](deploy/model-init/model-manifest.json).

## Reporting attribution issues

If you believe a component is missing attribution or is listed incorrectly,
please open an issue with the component name, version, and license evidence. Do
not include confidential or proprietary material.
