#!/bin/sh
# End-to-end smoke test against a running Docker stack.
#
#   ./scripts/smoke-test.sh
#   BASE_URL=http://localhost:3000 ./scripts/smoke-test.sh
#
# It uses the real HTTP API (no mocks): health, transcript upload, asynchronous
# indexing, and a hybrid search request.
set -eu
cd "$(dirname "$0")/.."

BASE_URL="${BASE_URL:-http://localhost:3000}"
API="$BASE_URL/api/v1"
FIXTURE="${1:-$PWD/scripts/fixtures/sample.vtt}"

fail() {
  echo "smoke-test: FAIL: $*" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"

echo "smoke-test: health"
curl -fsS "$BASE_URL/healthz" >/dev/null || fail "health endpoint is not reachable at $BASE_URL/healthz"
curl -fsS "$API/health" >/dev/null || fail "API health endpoint is not reachable"

echo "smoke-test: upload transcript"
response=$(curl -fsS -X POST "$API/transcripts" \
  -F "file=@$FIXTURE;type=text/vtt" \
  -F "title=Sutra smoke test" \
  -F "language=en")
# The response contains both the video id and the nested transcript id; take
# the first "id" field, which is the top-level video.
video_id=$(printf '%s' "$response" | grep -o '"id":"[^"]*"' | head -n1 | sed 's/^"id":"//;s/"$//')
[ -n "$video_id" ] || fail "could not read video id from upload response: $response"

echo "smoke-test: wait for indexing (video $video_id)"
attempt=0
while [ "$attempt" -lt 240 ]; do
  state=$(curl -fsS "$API/videos/$video_id/processing")
  status=$(printf '%s' "$state" | grep -o '"status":"[^"]*"' | head -n1 | sed 's/^"status":"//;s/"$//')
  case "$status" in
    completed) echo "smoke-test: indexing completed"; break ;;
    failed) fail "indexing failed: $state" ;;
  esac
  attempt=$((attempt + 1))
  sleep 1
done
[ "$status" = "completed" ] || fail "indexing did not complete within 240s: $state"

echo "smoke-test: hybrid search"
search=$(curl -fsS -X POST "$API/search" \
  -H 'Content-Type: application/json' \
  -d '{"query":"local embedding model","limit":5,"video_id":"'"$video_id"'"}')
printf '%s' "$search" | grep -q '"results"' || fail "search response has no results field: $search"
printf '%s' "$search" | grep -q '"chunk_id"' || fail "search returned no indexed chunks: $search"

echo "smoke-test: PASS"
