#!/bin/sh
# Download and verify the pinned embedding model.
#
# The manifest is the source of truth for the upstream revision, URLs and
# SHA-256 checksums. Files are written to a temporary path and only renamed
# into place after the checksum matches, so a partial download is never
# treated as a usable model.
set -eu

MANIFEST="${MODEL_MANIFEST:-/etc/sutra/model-manifest.json}"
MODEL_DIR="${MODEL_DIR:-/models}"

fail() {
  echo "model-init: ERROR: $*" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"

[ -r "$MANIFEST" ] || fail "manifest not readable: $MANIFEST"

model_name=$(jq -r '.model' "$MANIFEST")
revision=$(jq -r '.revision' "$MANIFEST")
model_license=$(jq -r '.license' "$MANIFEST")
directory=$(jq -r '.directory' "$MANIFEST")
dimensions=$(jq -r '.dimensions' "$MANIFEST")

[ "$model_name" != "null" ] || fail "manifest is missing .model"
[ "$revision" != "null" ] || fail "manifest is missing .revision"
[ "$directory" != "null" ] || fail "manifest is missing .directory"

if [ -n "${MODEL_REVISION:-}" ] && [ "$MODEL_REVISION" != "$revision" ]; then
  fail "MODEL_REVISION=${MODEL_REVISION} does not match the pinned manifest revision ${revision}" \
       "Update deploy/model-init/model-manifest.json instead of overriding only the revision."
fi

dest="$MODEL_DIR/$directory"
ready="$dest/.ready"
mkdir -p "$dest"

echo "model-init: model=$model_name revision=$revision dimensions=$dimensions license=$model_license"
echo "model-init: destination=$dest"

# Invalidate the readiness marker while files are being validated or replaced.
rm -f "$ready"

count=$(jq '.files | length' "$MANIFEST")
[ "$count" -gt 0 ] || fail "manifest contains no files"
i=0
while [ "$i" -lt "$count" ]; do
  entry=$(jq -c ".files[$i]" "$MANIFEST")
  name=$(echo "$entry" | jq -r '.name')
  url=$(echo "$entry" | jq -r '.url')
  expected=$(echo "$entry" | jq -r '.sha256')
  expected_size=$(echo "$entry" | jq -r '.size')
  target="$dest/$name"
  i=$((i + 1))

  [ "$name" != "null" ] || fail "manifest file $i has no name"
  [ "$url" != "null" ] || fail "manifest file $name has no url"
  [ "$expected" != "null" ] || fail "manifest file $name has no sha256"

  if [ -f "$target" ]; then
    actual=$(sha256sum "$target" | awk '{print $1}')
    if [ "$actual" = "$expected" ]; then
      echo "model-init: $name already present and verified"
      continue
    fi
    echo "model-init: $name checksum mismatch; replacing it"
    rm -f "$target"
  fi

  tmp="$target.download.$$"
  trap 'rm -f "$tmp"' EXIT INT TERM
  echo "model-init: downloading $name"
  if ! curl --fail --location --silent --show-error \
      --retry 5 --retry-delay 5 --retry-all-errors \
      --connect-timeout 30 --max-time 1800 \
      --output "$tmp" "$url"; then
    rm -f "$tmp"
    fail "download failed for $name from $url. Check network access to huggingface.co and retry 'docker compose up'."
  fi

  actual=$(sha256sum "$tmp" | awk '{print $1}')
  if [ "$actual" != "$expected" ]; then
    rm -f "$tmp"
    fail "checksum mismatch for $name: expected $expected, got $actual. Delete the model volume or retry; do not use this file."
  fi
  actual_size=$(wc -c < "$tmp" | tr -d ' ')
  if [ "$actual_size" != "$expected_size" ]; then
    rm -f "$tmp"
    fail "size mismatch for $name: expected $expected_size bytes, got $actual_size"
  fi

  chmod 0644 "$tmp"
  mv "$tmp" "$target"
  trap - EXIT INT TERM
  echo "model-init: verified $name ($actual_size bytes)"
done

chmod 0755 "$dest"
cat > "$ready.tmp.$$" <<EOF
model=$model_name
revision=$revision
license=$model_license
dimensions=$dimensions
verified_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF
chmod 0644 "$ready.tmp.$$"
mv "$ready.tmp.$$" "$ready"

echo "model-init: ready ($model_name @ $revision)"
