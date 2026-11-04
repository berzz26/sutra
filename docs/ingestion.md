# Ingestion

Sutra accepts two sources:

1. **YouTube videos with existing subtitles.** Only the subtitle track is
   downloaded; audio and video media are never fetched.
2. **Uploaded WebVTT (`.vtt`) or SubRip (`.srt`) files.**

In both cases, raw cues are stored in PostgreSQL, and an asynchronous job chunks
and embeds the transcript.

## YouTube ingestion

### Accepted URLs

Only HTTPS URLs on these hosts are accepted:

- `youtube.com`, `www.youtube.com`, `m.youtube.com`, `music.youtube.com`
- `youtu.be`, `www.youtu.be`

Supported path forms:

```text
https://www.youtube.com/watch?v=VIDEO_ID
https://m.youtube.com/watch?v=VIDEO_ID
https://music.youtube.com/watch?v=VIDEO_ID
https://youtu.be/VIDEO_ID
https://www.youtube.com/shorts/VIDEO_ID
https://www.youtube.com/live/VIDEO_ID
https://www.youtube.com/embed/VIDEO_ID
```

Rejected:

- HTTP URLs.
- URLs with credentials or custom ports.
- Non-YouTube domains.
- Playlist URLs and watch URLs containing a `list` parameter.
- Unknown YouTube paths.
- Video IDs longer than 32 characters or containing anything other than
  letters, digits, `-`, and `_`.

CSS/JS-injected tracking parameters are stripped. The stored canonical URL is
`https://www.youtube.com/watch?v=VIDEO_ID`.

### Flow

```text
POST /api/v1/videos
  -> validate and canonicalize URL
  -> create or reuse videos row (dedup by YouTube video ID)
  -> yt-dlp --dump-single-json --skip-download
  -> choose a subtitle track
  -> yt-dlp download of that subtitle track only
  -> parse VTT/SRT into cues
  -> transaction: video metadata + transcript + cues
  -> insert embed job
  -> return video row
```

The HTTP request waits for metadata and subtitle extraction. Chunking and
embedding happen later in the in-process worker. The UI polls
`GET /api/v1/videos/:id/processing` until the video is searchable.

`yt-dlp` runs as a subprocess with an argument array, not through a shell. Each
command is limited by `YTDLP_TIMEOUT_SECONDS` (default 120). Concurrent
extractions are limited by `TRANSCRIPT_MAX_CONCURRENT_INGESTIONS` (default 2).
The same video is serialized within one API process so concurrent retries cannot
duplicate its cues.

### Subtitle selection

Subtitle language keys are preserved exactly as `yt-dlp` reports them,
including keys such as `en-orig` and `pt-BR`.

When `language` is supplied:

1. A manual track with the exact requested key wins.
2. For a regional request (for example `en-US`), an exact regional automatic
   track wins over a generic original track.
3. Manual tracks in the same language family come before automatic tracks.
4. Original automatic captions come before translated automatic captions.
5. An unavailable requested language returns `LANGUAGE_UNAVAILABLE`; Sutra does
   not silently substitute a different language.

When `language` is empty or omitted:

1. Prefer English: `en`, then `en-US`, then `en-GB`.
2. Otherwise prefer the first manual track in deterministic key order.
3. Otherwise prefer an original automatic caption track.
4. Otherwise return `NO_TRANSCRIPT`.

The API can also ingest an exact discovered track:

```json
{
  "url": "https://www.youtube.com/watch?v=VIDEO_ID",
  "language": "en",
  "subtitle_key": "en",
  "subtitle_source": "manual"
}
```

`subtitle_source` is `manual` or `automatic` (empty is treated as `manual` with
an automatic fallback). `subtitle_key` is the exact `language` value returned by
the discovery endpoint.

Only VTT/SRT-compatible tracks are considered. A video with no compatible
captions returns `NO_TRANSCRIPT`.

### Discovering subtitle tracks

```sh
curl 'http://localhost:3000/api/v1/videos/subtitles?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DVIDEO_ID'
```

The response contains metadata and two arrays:

- `subtitles`: manual tracks plus original automatic tracks.
- `translated_subtitles`: automatic tracks that are translations.

Each option includes the exact `language` key, display `name`, `source`
(`manual`/`automatic`), available `formats`, and `original`/`translated` flags.
Use `language` and `source` as `subtitle_key` and `subtitle_source` when
ingesting.

### Idempotency and retries

| Existing status | Behavior on a new request |
|---|---|
| `ready` | Return the existing video and transcript immediately. |
| `no_transcript` | Return the existing terminal row. |
| `fetching_transcript`, `processing` | Return the in-flight row; do not start a second extraction. |
| `failed` | Retry extraction and replace the transcript/cues on success. |

A failed extraction records `failed` (or `no_transcript` for a missing or
unavailable language track) on the video row. Submitting the same URL again is
the retry mechanism.

## Transcript uploads

```sh
curl -X POST http://localhost:3000/api/v1/transcripts \
  -F 'file=@talk.vtt;type=text/vtt' \
  -F 'title=Example talk' \
  -F 'language=en' \
  -F 'original_url=https://www.youtube.com/watch?v=VIDEO_ID'
```

Multipart fields:

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | `.vtt` or `.srt`; maximum size is `TRANSCRIPT_MAX_UPLOAD_MB` (20 MiB default). |
| `title` | No | Falls back to the uploaded filename. |
| `language` | No | Stored as-is; no validation or normalization. |
| `original_url` | No | If provided, must be an accepted HTTPS YouTube URL. It enables timestamp links. |

Validation:

- The extension must be `.vtt` or `.srt` (case-insensitive).
- The bytes are parsed as the declared format and content-sniffed, so a `.srt`
  file containing VTT (or vice versa) is rejected.
- Invalid timestamps, empty transcripts, and unsupported content return a
  structured error.
- The body size limit is the configured upload limit plus 1 MiB of multipart
  overhead.

The SHA-256 of the file content is the deduplication key. Re-uploading identical
bytes returns the existing video even if the filename or title differs. Different
content always creates a separate video.

## Processing and progress

After persistence, ingestion inserts one `embed` job. The worker:

1. Groups complete cues into overlapping chunks (180-word target, 80-word
   minimum, 250-word maximum, up to 30 words of overlap).
2. Embeds chunks in batches with the local BGE model.
3. Upserts chunks and vectors in a transaction.

Progress is available at:

```sh
curl http://localhost:3000/api/v1/videos/VIDEO_ID/processing
```

Example:

```json
{
  "video_id": "4d5e...",
  "status": "completed",
  "stage": "complete",
  "progress_percent": 100,
  "cue_count": 42,
  "embedding_count": 6,
  "chunk_count": 6
}
```

`status` is `pending`, `processing`, `completed`, or `failed`. When failed,
`last_error` contains the message.

To re-run processing manually (for example after changing chunking code):

```sh
curl -X POST http://localhost:3000/api/v1/videos/VIDEO_ID/process
```

Or enqueue existing transcripts that have cues but no chunks:

```sh
cd api
go run ./cmd/backfill
```

## Optional artifact storage

By default, original subtitle files are not retained as objects. Raw cues and
derived chunks are always in PostgreSQL.

When `ARTIFACT_STORE_ENABLED=true`, the original file is uploaded to the
configured S3-compatible bucket under a key shaped like:

```text
transcripts/<video-id>/<transcript-id>/original.<vtt|srt>
```

The object key is stored in `transcripts.artifact_key`. If the database
transaction fails after upload, the object is deleted on a best-effort basis. If
a video is replaced, superseded artifacts are deleted. Deleting a video removes
its database rows first and then its artifacts; an artifact deletion failure is
reported in logs.

The bucket must already exist. Sutra does not create it. See
[Configuration](configuration.md#optional-artifact-store).

## Error codes

| Code | Typical HTTP status | Meaning |
|---|---|---|
| `INVALID_YOUTUBE_URL` | 400 | Malformed URL, non-HTTPS, credentials/port, bad video ID. |
| `UNSUPPORTED_URL` | 400 | Non-YouTube domain or unknown YouTube path. |
| `PLAYLIST_NOT_SUPPORTED` | 400 | Playlist or watch URL with `list`. |
| `VIDEO_UNAVAILABLE` | 422 | Removed, terminated, or otherwise unavailable. |
| `VIDEO_INACCESSIBLE` | 422 | Private, members-only, age-restricted, or login required. |
| `NO_TRANSCRIPT` | 422 | No compatible subtitle track exists. |
| `LANGUAGE_UNAVAILABLE` | 422 | Requested track/language is not available. |
| `YTDLP_NOT_INSTALLED` | 400 | Binary not found. |
| `YTDLP_TIMEOUT` | 400 | Command exceeded `YTDLP_TIMEOUT_SECONDS`. |
| `YTDLP_RATE_LIMITED` | 429 | YouTube returned a rate limit. |
| `YTDLP_EXTRACTION_FAILED` | 400 | Other yt-dlp failure. |
| `INVALID_SUBTITLE_FILE` | 400 | Parse error or content/extension mismatch. |
| `INVALID_VTT`, `INVALID_SRT` | 400 | Format-specific parse error. |
| `UNSUPPORTED_FILE_TYPE` | 400 | Missing file or non-VTT/SRT extension. |
| `FILE_TOO_LARGE` | 413 | Upload exceeds the configured limit. |
| `EMPTY_TRANSCRIPT` | 400 | No usable cues. |
| `INVALID_TIMESTAMP` | 400 | Timestamp outside the accepted range. |
| `MINIO_UPLOAD_FAILED` | 500 | Artifact store operation failed. |
| `DATABASE_PERSISTENCE_FAILED` | 500 | Database transaction or query failed. |
| `VIDEO_NOT_FOUND` | 404 | Unknown video ID. |

## Operational caveats

- YouTube extraction is inherently fragile. A change on YouTube's side, an
  outdated yt-dlp build, or a challenge page can break ingestion.
- Cookies, browser impersonation, and proxy configuration are not exposed. A
  video that requires login cannot be ingested.
- Rate limiting from YouTube is passed through as `YTDLP_RATE_LIMITED`. Reduce
  concurrency and retry later.
- The Docker image pins yt-dlp and Deno; upgrade them together when needed and
  update the checksums in `api/Dockerfile` and `THIRD_PARTY_NOTICES.md`.
- Ensure you have the right to download and use the subtitles you ingest, and
  comply with YouTube's terms and the content's license.
