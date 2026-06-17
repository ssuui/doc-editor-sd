#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODE="${1:-release}"

case "${MODE}" in
  dev)
    exec "${ROOT_DIR}/build_dev.sh"
    ;;
  release)
    exec "${ROOT_DIR}/build_release.sh"
    ;;
  *)
    echo "usage: $0 [dev|release]" >&2
    exit 1
    ;;
esac
