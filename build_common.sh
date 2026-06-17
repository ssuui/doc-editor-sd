#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="${ROOT_DIR}/web"
BACKEND_DIR="${ROOT_DIR}/service"
STATIC_DIR="${BACKEND_DIR}/static_resources"
RELEASE_DIR="${ROOT_DIR}/release"
APP_NAME="doc-publish-server"
GO_BUILD_FLAGS=(-trimpath -ldflags=-s\ -w)

ensure_dir() {
  mkdir -p "$1"
}

reset_dir() {
  rm -rf "$1"
  mkdir -p "$1"
}

copy_tree() {
  local src="$1"
  local dst="$2"
  reset_dir "${dst}"
  if [ -d "${src}" ]; then
    cp -R "${src}/." "${dst}/"
  fi
}

ensure_frontend_deps() {
  if [ ! -d "${FRONTEND_DIR}/node_modules" ]; then
    cd "${FRONTEND_DIR}"
    if [ -f package-lock.json ]; then
      npm ci
    else
      npm install
    fi
  fi
}

ensure_service_layout() {
  ensure_dir "${BACKEND_DIR}/bin"
  ensure_dir "${BACKEND_DIR}/config"
  ensure_dir "${BACKEND_DIR}/global_theme"
  ensure_dir "${BACKEND_DIR}/source_root"
  ensure_dir "${STATIC_DIR}"
  ensure_dir "${BACKEND_DIR}/build_temp/main_site_out"
  ensure_dir "${BACKEND_DIR}/build_temp/book_cache"
  ensure_dir "${BACKEND_DIR}/build_temp/full_package"
  ensure_dir "${BACKEND_DIR}/publish_records"
}

build_frontend() {
  ensure_service_layout
  ensure_frontend_deps
  cd "${FRONTEND_DIR}"
  npm run build
  copy_tree "${FRONTEND_DIR}/dist" "${STATIC_DIR}"
}

build_backend() {
  ensure_service_layout
  cd "${BACKEND_DIR}"
  CGO_ENABLED=0 go build "${GO_BUILD_FLAGS[@]}" -o "${APP_NAME}" ./cmd
}

ensure_release_layout() {
  reset_dir "${RELEASE_DIR}"
  ensure_dir "${RELEASE_DIR}/runtime/bin"
  ensure_dir "${RELEASE_DIR}/runtime/global_theme"
  ensure_dir "${RELEASE_DIR}/runtime/static_resources"
  ensure_dir "${RELEASE_DIR}/config"
  ensure_dir "${RELEASE_DIR}/source_root"
  ensure_dir "${RELEASE_DIR}/var/build_temp/main_site_out"
  ensure_dir "${RELEASE_DIR}/var/build_temp/book_cache"
  ensure_dir "${RELEASE_DIR}/var/build_temp/full_package"
  ensure_dir "${RELEASE_DIR}/var/publish_records"
}

rewrite_release_system_config() {
  local path="$1"
  python3 - "$path" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
updates = {
    "source_root_path": "./source_root",
    "hugo_bin_path": "./runtime/bin/hugo-extended",
    "global_theme_path": "./runtime/global_theme",
    "build_temp_root": "./var/build_temp",
    "publish_record_path": "./var/publish_records",
}

lines = path.read_text(encoding="utf-8").splitlines()
seen = set()
out = []
for line in lines:
    stripped = line.strip()
    replaced = False
    for key, value in updates.items():
      prefix = f"{key}:"
      if line.startswith(prefix):
          out.append(f'{key}: "{value}"')
          seen.add(key)
          replaced = True
          break
    if not replaced:
        out.append(line)

for key, value in updates.items():
    if key not in seen:
        out.insert(0, f'{key}: "{value}"')

path.write_text("\n".join(out) + "\n", encoding="utf-8")
PY
}

write_release_helpers() {
  cat > "${RELEASE_DIR}/run.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${BASE_DIR}"

mkdir -p \
  ./runtime/bin \
  ./runtime/static_resources \
  ./runtime/global_theme \
  ./config \
  ./source_root \
  ./var/build_temp/main_site_out \
  ./var/build_temp/book_cache \
  ./var/build_temp/full_package \
  ./var/publish_records

chmod +x ./runtime/doc-publish-server ./runtime/bin/hugo-extended 2>/dev/null || true
APP_STATIC_DIR="./runtime/static_resources" exec ./runtime/doc-publish-server
EOF
  chmod +x "${RELEASE_DIR}/run.sh"

  cat > "${RELEASE_DIR}/verify.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"
PORT="${PORT:-18080}"
TOKEN_FILE="${BASE_DIR}/.verify_token"
LOG_FILE="${BASE_DIR}/verify-server.log"
PID_FILE="${BASE_DIR}/verify-server.pid"

cleanup() {
  if [ -f "${PID_FILE}" ]; then
    pid="$(cat "${PID_FILE}")"
    if kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
      wait "${pid}" 2>/dev/null || true
    fi
    rm -f "${PID_FILE}"
  fi
  rm -f "${TOKEN_FILE}"
  if [ -f "${BASE_DIR}/config/system.yaml.bak" ]; then
    mv "${BASE_DIR}/config/system.yaml.bak" "${BASE_DIR}/config/system.yaml"
  fi
}
trap cleanup EXIT

cd "${BASE_DIR}"
mkdir -p \
  ./runtime/bin \
  ./runtime/static_resources \
  ./runtime/global_theme \
  ./config \
  ./source_root \
  ./var/build_temp/main_site_out \
  ./var/build_temp/book_cache \
  ./var/build_temp/full_package \
  ./var/publish_records

cp config/system.yaml config/system.yaml.bak
python3 - <<'PY'
from pathlib import Path
path = Path("config/system.yaml")
text = path.read_text(encoding="utf-8")
lines = text.splitlines()
out = []
replaced = False
for line in lines:
    if line.startswith("http_port:"):
        out.append(f"http_port: {__import__('os').environ.get('PORT', '18080')}")
        replaced = True
    else:
        out.append(line)
if not replaced:
    out.insert(0, f"http_port: {__import__('os').environ.get('PORT', '18080')}")
path.write_text("\n".join(out) + "\n", encoding="utf-8")
PY

chmod +x ./runtime/doc-publish-server ./runtime/bin/hugo-extended
APP_STATIC_DIR="./runtime/static_resources" ./runtime/doc-publish-server > "${LOG_FILE}" 2>&1 &
echo $! > "${PID_FILE}"

ready=0
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/login.html" >/dev/null; then
    ready=1
    break
  fi
  sleep 1
done

if [ "${ready}" -ne 1 ]; then
  echo "server did not start, see ${LOG_FILE}" >&2
  exit 1
fi

login_payload="$(python3 - <<'PY'
from pathlib import Path
import json

text = Path("config/system.yaml").read_text(encoding="utf-8")
username = "admin"
password = ""
in_auth = False
for raw in text.splitlines():
    line = raw.rstrip()
    if not line or line.lstrip().startswith("#"):
        continue
    if not line.startswith(" ") and line.endswith(":"):
        in_auth = line.strip() == "auth:"
        continue
    if not in_auth:
        continue
    stripped = line.strip()
    if stripped.startswith("admin_username:"):
        username = stripped.split(":", 1)[1].strip().strip('"')
    if stripped.startswith("admin_password:"):
        password = stripped.split(":", 1)[1].strip().strip('"')

print(json.dumps({"username": username, "password": password}, ensure_ascii=False))
PY
)"
login_resp="$(curl -fsS -X POST "http://127.0.0.1:${PORT}/api/login" \
  -H 'Content-Type: application/json' \
  -d "${login_payload}")"
token="$(printf '%s' "${login_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])')"
printf '%s' "${token}" > "${TOKEN_FILE}"

curl -fsS "http://127.0.0.1:${PORT}/api/build/check-hugo?token=${token}" >/dev/null
curl -fsS "http://127.0.0.1:${PORT}/api/fs/book/list?token=${token}" >/dev/null

echo "verify ok: http://127.0.0.1:${PORT}"
EOF
  chmod +x "${RELEASE_DIR}/verify.sh"

  cat > "${RELEASE_DIR}/README.txt" <<'EOF'
Release package layout:
- runtime/: binary, Hugo runtime, theme, frontend static files
- config/: service config files kept outside runtime
- source_root/: editable document source data
- var/: build temp data and publish records
- run.sh: start the service in-place
- verify.sh: boot the service on a temporary port and verify login + Hugo detection + book listing

Quick start:
1. chmod +x ./run.sh ./verify.sh ./runtime/doc-publish-server ./runtime/bin/hugo-extended
2. ./verify.sh
3. ./run.sh
EOF
}

package_release() {
  ensure_service_layout
  ensure_release_layout

  cp "${BACKEND_DIR}/${APP_NAME}" "${RELEASE_DIR}/runtime/${APP_NAME}"
  copy_tree "${BACKEND_DIR}/bin" "${RELEASE_DIR}/runtime/bin"
  copy_tree "${BACKEND_DIR}/global_theme" "${RELEASE_DIR}/runtime/global_theme"
  copy_tree "${BACKEND_DIR}/static_resources" "${RELEASE_DIR}/runtime/static_resources"
  copy_tree "${BACKEND_DIR}/config" "${RELEASE_DIR}/config"
  copy_tree "${BACKEND_DIR}/source_root" "${RELEASE_DIR}/source_root"

  if [ -f "${RELEASE_DIR}/config/system.yaml" ]; then
    rewrite_release_system_config "${RELEASE_DIR}/config/system.yaml"
  fi
  if [ -f "${RELEASE_DIR}/config/system.yaml.example" ]; then
    rewrite_release_system_config "${RELEASE_DIR}/config/system.yaml.example"
  fi

  write_release_helpers
}
