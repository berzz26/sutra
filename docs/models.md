# Models and pinned artifacts

Sutra embeds and searches locally. No transcript text or query is sent to an
external AI service. The model, runtime, and extraction tools are pinned and
checksum-verified so a clean install is reproducible.

## Embedding model

| Property | Value |
|---|---|
| Model | `BAAI/bge-small-en-v1.5` |
| Hugging Face revision | `5c38ec7c405ec4b44b94cc5a9bb96e735b38267a` |
| Source | <https://huggingface.co/BAAI/bge-small-en-v1.5> |
| License | MIT (per the model card) |
| Dimensions | 384 |
| Max tokens | 512 |
| Pooling | Mask-aware mean pooling, then L2 normalization |
| Model file | `onnx/model.onnx` from the pinned revision |
| Tokenizer file | `tokenizer.json` from the pinned revision |

The model is a small English bi-encoder chosen to run on CPU in a self-hosted
single-node deployment. It is not a reranker or a generative model.

### Pinned checksums

The source of truth is
[`deploy/model-init/model-manifest.json`](../deploy/model-init/model-manifest.json):

| File | SHA-256 | Size |
|---|---|---|
| `model.onnx` | `828e1496d7fabb79cfa4dcd84fa38625c0d3d21da474a00f08db0f559940cf35` | 133,093,490 bytes |
| `tokenizer.json` | `d241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66` | 711,396 bytes |

The repository tracks only the tokenizer (`api/embedding/models/bge-small-en-v1.5/tokenizer.json`)
for host development convenience. Model weights (`*.onnx`) and native runtime
archives are ignored and never committed.

## How the model is provisioned

`docker compose up` starts a one-shot `model-init` service before the API:

1. Reads the manifest baked into the `model-init` image.
2. Creates `/models/bge-small-en-v1.5` in the `sutra_model-data` volume.
3. Removes any existing `.ready` marker.
4. For each manifest file: if it exists and its SHA-256 matches, skip it;
   otherwise download to a temporary file, verify SHA-256 and byte size, then
   atomically rename it into place.
5. Writes `.ready` with the model, revision, license, dimensions, and
   verification timestamp.

A partial or corrupted download is never moved into place. A failed download
fails the service, so Compose does not start the API against an incomplete
model. The API also performs a warm-up inference at startup and refuses to start
if the ONNX session cannot be created.

`MODEL_REVISION` is an optional guard. If set, it must exactly match the
manifest revision; otherwise `model-init` fails. It is not a way to select an
arbitrary revision. To upgrade, update the manifest, checksums, and this
document together.

### Manual verification

Inside the volume or a local checkout:

```sh
sha256sum model.onnx tokenizer.json
```

Compare against the table above. To inspect the manifest:

```sh
jq . deploy/model-init/model-manifest.json
```

### Local development

`make fetch-model` or `./scripts/fetch-model.sh` downloads the same pinned files
into `api/embedding/models/bge-small-en-v1.5/`. It reuses the same manifest and
verification logic as the container.

## ONNX Runtime

| Property | Value |
|---|---|
| Version | `1.31.1` |
| Distribution | `onnxruntime-linux-x64-1.31.1.tgz` |
| Source | <https://github.com/microsoft/onnxruntime/releases/tag/v1.31.1> |
| Archive SHA-256 | `c849e36a1ef3e22d4ff217b9d1ce4a3cf5b9467b975610913ae959ad5b16bdc9` |
| Container path | `/opt/onnxruntime/lib/libonnxruntime.so` |
| License | MIT; see `third_party/licenses/` |

The Go binding uses cgo and `dlopen`, so the API image builds with
`CGO_ENABLED=1` and uses a Debian glibc runtime. The official upstream Linux
build is glibc, not musl; Alpine is not a supported API runtime.

For host development:

```sh
make fetch-onnxruntime
# installs into api/third_party/onnxruntime/onnxruntime-linux-x64-1.31.1/
```

## yt-dlp and Deno

| Tool | Version | Source | SHA-256 |
|---|---|---|---|
| yt-dlp | `2026.10.26` | <https://github.com/yt-dlp/yt-dlp/releases/tag/2026.10.26> | `767ba9dce7052aa196f8b12652f433d76de128ba168966fa84b6394d5e6c6795` |
| Deno | `2.9.8` | <https://github.com/denoland/deno/releases/tag/v2.9.8> | `e890975bc2549807bbcc0b584ffafda72cc7c93349dd99c80483dd784f6c4609` |

The API image downloads `yt-dlp_linux`, which bundles `yt-dlp-ejs`, and installs
Deno as the JavaScript runtime used for YouTube challenge solving. Both are
verified at image build time. They are only needed for YouTube ingestion;
uploads and search do not use them.

yt-dlp is Unlicense; Deno is MIT. See
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).

## Supply-chain integrity

Pinned artifacts are verified at two points:

| Artifact | Verified when |
|---|---|
| Model + tokenizer | `model-init` startup (SHA-256 + size) |
| ONNX Runtime | API image build (SHA-256) |
| yt-dlp | API image build (SHA-256), then `--version` runs |
| Deno | API image build (SHA-256), then `--version` runs |

Container base images are pinned by tag and digest resolution is delegated to
the registry. For a stricter deployment, pin image digests in Compose and the
Dockerfiles and mirror the artifacts internally.

## Changing the model or chunking

The model and index shape are coupled:

- `EMBEDDING_DIMENSIONS` must match the `vector(384)` column and the model's
  output dimension.
- A different dimension requires a new migration that changes the column type
  and rebuilds the HNSW index, plus reindexing every video.
- A different tokenizer or model file must still provide the ONNX inputs
  `input_ids`, `attention_mask`, and `token_type_ids`, a single token output,
  and a compatible `tokenizer.json`.
- Chunk target/minimum/maximum/overlap are compiled constants. Changing them
  requires a code change and a new processing version.

After changing the model, tokenizer, or chunking version:

1. Update `deploy/model-init/model-manifest.json` with the new revision, URLs,
   checksums, and sizes.
2. Update `THIRD_PARTY_NOTICES.md` and this document.
3. Rebuild and restart the stack so `model-init` refreshes the model volume.
4. Enqueue all ready videos for reprocessing:

   ```sql
   INSERT INTO jobs (video_id, kind, status, run_after)
   SELECT id, 'embed', 'queued', now()
   FROM videos
   WHERE status = 'ready'
   ON CONFLICT DO NOTHING;
   ```

   Workers will reprocess videos whose stored processing/model version differs
   from the current configuration. Alternatively call
   `POST /api/v1/videos/:id/process` per video.

5. Verify processing completes and run a search smoke test.

`go run ./cmd/backfill` only enqueues videos that have cues but **no** chunks. It
does not re-embed an already indexed library.

## Model license and redistribution

The BGE-small-en-v1.5 model card declares MIT. Sutra does not redistribute the
weights in Git or in its container images; `model-init` downloads them from the
pinned Hugging Face revision. If you mirror or redistribute the model, retain
the upstream license and attribution.

Third-party license texts and notices are collected in
[`third_party/licenses/`](../third_party/licenses/) and summarized in
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).
