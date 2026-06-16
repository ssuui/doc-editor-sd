#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="${ROOT_DIR}/frontend-project"
BACKEND_DIR="${ROOT_DIR}/go-backend"
STATIC_DIR="${BACKEND_DIR}/static_resources"

if [ ! -d "${FRONTEND_DIR}/node_modules" ]; then
  cd "${FRONTEND_DIR}"
  npm install
fi

cd "${FRONTEND_DIR}"
npm run build

rm -rf "${STATIC_DIR}"
mkdir -p "${STATIC_DIR}"
cp -R "${FRONTEND_DIR}/dist/." "${STATIC_DIR}/"

cd "${BACKEND_DIR}"
go mod tidy
go build -o doc-publish-server ./cmd
