#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
repository=${1:?image repository required}
fingerprint=$(bash devenv/ci-fingerprint.sh)
digest=${2:-}
if [[ -z "$digest" ]]; then
  digest=$(docker buildx imagetools inspect --format '{{.Manifest.Digest}}' "$repository:ci-$fingerprint")
fi
if [[ ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo "Invalid image digest for $repository" >&2
  exit 1
fi

image="$repository@$digest"
docker pull --platform linux/amd64 "$image" >&2
actual=$(docker image inspect --format '{{index .Config.Labels "io.github.haikoschol.cats-of-asia-reloaded.ci-fingerprint"}}' "$image")
if [[ "$actual" != "$fingerprint" ]]; then
  echo "CI image fingerprint mismatch for $image" >&2
  exit 1
fi
printf '%s\n' "$image"
