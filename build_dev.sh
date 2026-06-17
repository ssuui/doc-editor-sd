#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${ROOT_DIR}/build_common.sh"

build_frontend
build_backend

echo "dev build ready: ${BACKEND_DIR}"
