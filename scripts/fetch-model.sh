#!/bin/sh
# Local development helper: download the pinned model into the api/ tree.
#
# Docker users do not need this; compose runs deploy/model-init automatically.
set -eu
cd "$(dirname "$0")/.."

MODEL_MANIFEST="${MODEL_MANIFEST:-$PWD/deploy/model-init/model-manifest.json}"
MODEL_DIR="${MODEL_DIR:-$PWD/api/embedding/models}"
export MODEL_MANIFEST MODEL_DIR

exec sh "$PWD/deploy/model-init/fetch-model.sh"
