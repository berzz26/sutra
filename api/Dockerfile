# syntax=docker/dockerfile:1
#
# CGO is required because github.com/yalue/onnxruntime_go loads the ONNX
# Runtime shared library through cgo/dlopen. The runtime image is Debian
# (glibc) because the official upstream ONNX Runtime Linux builds are glibc
# binaries; Alpine/musl is not a supported runtime for this stack.

FROM golang:1.27-bookworm AS build
WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sutra ./cmd/server \
    && CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/backfill ./cmd/backfill


FROM debian:bookworm-slim AS runtime

# Keep these versions and checksums in sync with THIRD_PARTY_NOTICES.md.
ARG ORT_VERSION=1.31.1
ARG ORT_SHA256=c849e36a1ef3e22d4ff217b9d1ce4a3cf5b9467b975610913ae959ad5b16bdc9
ARG YTDLP_VERSION=2026.10.26
ARG YTDLP_SHA256=767ba9dce7052aa196f8b12652f433d76de128ba168966fa84b6394d5e6c6795
ARG DENO_VERSION=2.9.8
ARG DENO_SHA256=e890975bc2549807bbcc0b584ffafda72cc7c93349dd99c80483dd784f6c4609

ENV DEBIAN_FRONTEND=noninteractive \
    ONNXRUNTIME_HOME=/opt/onnxruntime \
    LD_LIBRARY_PATH=/opt/onnxruntime/lib

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        libstdc++6 \
        tar \
        unzip \
    && rm -rf /var/lib/apt/lists/*

# Official ONNX Runtime CPU distribution, pinned and checksum-verified.
RUN curl --fail --location --silent --show-error \
        --output /tmp/onnxruntime.tgz \
        "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz" \
    && echo "${ORT_SHA256}  /tmp/onnxruntime.tgz" | sha256sum -c - \
    && mkdir -p "${ONNXRUNTIME_HOME}" \
    && tar -xzf /tmp/onnxruntime.tgz --strip-components=1 -C "${ONNXRUNTIME_HOME}" \
    && rm -f /tmp/onnxruntime.tgz \
    && test -f "${ONNXRUNTIME_HOME}/lib/libonnxruntime.so"

# yt-dlp's official PyInstaller build bundles yt-dlp-ejs. It still needs an
# external JavaScript runtime for YouTube challenge solving; Deno is the
# upstream-recommended option and is enabled by default by yt-dlp.
RUN curl --fail --location --silent --show-error \
        --output /usr/local/bin/yt-dlp \
        "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/yt-dlp_linux" \
    && echo "${YTDLP_SHA256}  /usr/local/bin/yt-dlp" | sha256sum -c - \
    && chmod 0755 /usr/local/bin/yt-dlp \
    && /usr/local/bin/yt-dlp --version

RUN curl --fail --location --silent --show-error \
        --output /tmp/deno.zip \
        "https://github.com/denoland/deno/releases/download/v${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
    && echo "${DENO_SHA256}  /tmp/deno.zip" | sha256sum -c - \
    && unzip -o -q /tmp/deno.zip -d /usr/local/bin \
    && chmod 0755 /usr/local/bin/deno \
    && rm -f /tmp/deno.zip \
    && deno --version

RUN groupadd --system --gid 10001 app \
    && useradd --system --uid 10001 --gid app --create-home --home-dir /home/app --shell /usr/sbin/nologin app

WORKDIR /app
COPY --from=build /out/sutra /app/sutra
COPY --from=build /out/backfill /app/backfill

USER app

EXPOSE 8080
ENTRYPOINT ["/app/sutra"]
