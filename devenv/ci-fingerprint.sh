#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
# Image inputs live in these files and in devenv/ (including imported modules).
{
  printf '%s\n' 'linux/amd64'
  sha256sum devenv.nix devenv.yaml devenv.lock .github/workflows/ci-image.yml
  find devenv \( -type f -o -type l \) -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
} | sha256sum | cut -d ' ' -f 1
