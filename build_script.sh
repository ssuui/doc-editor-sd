#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="${ROOT_DIR}/web"
BACKEND_DIR="${ROOT_DIR}/service"
STATIC_DIR="${BACKEND_DIR}/static_resources"
RELEASE_DIR="${ROOT_DIR}/release"
APP_NAME="doc-publish-server"

GO_BUILD_FLAGS=(-trimpath -ldflags=-s\ -w)

copy_tree() {
  local src="$1"
  local dst="$2"
  rm -rf "${dst}"
  mkdir -p "${dst}"
  cp -R "${src}/." "${dst}/"
}

write_release_helpers() {
  cat > "${RELEASE_DIR}/run.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${BASE_DIR}"

chmod +x ./doc-publish-server ./bin/hugo-extended 2>/dev/null || true
exec ./doc-publish-server
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

chmod +x ./doc-publish-server ./bin/hugo-extended
./doc-publish-server > "${LOG_FILE}" 2>&1 &
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
- run.sh: start the service in-place
- verify.sh: boot the service on a temporary port and verify login + Hugo detection + book listing
- static_resources/: frontend static files served by the Go binary
- bin/hugo-extended: Hugo runtime used during publish

Quick start:
1. chmod +x ./run.sh ./verify.sh ./doc-publish-server ./bin/hugo-extended
2. ./verify.sh
3. ./run.sh
EOF
}

if [ ! -d "${FRONTEND_DIR}/node_modules" ]; then
  cd "${FRONTEND_DIR}"
  if [ -f package-lock.json ]; then
    npm ci
  else
    npm install
  fi
fi

cd "${FRONTEND_DIR}"
npm run build

copy_tree "${FRONTEND_DIR}/dist" "${STATIC_DIR}"

cd "${BACKEND_DIR}"
CGO_ENABLED=0 go build "${GO_BUILD_FLAGS[@]}" -o "${APP_NAME}" ./cmd

rm -rf "${RELEASE_DIR}"
mkdir -p "${RELEASE_DIR}"
cp "${BACKEND_DIR}/${APP_NAME}" "${RELEASE_DIR}/${APP_NAME}"
copy_tree "${BACKEND_DIR}/bin" "${RELEASE_DIR}/bin"
copy_tree "${BACKEND_DIR}/config" "${RELEASE_DIR}/config"
copy_tree "${BACKEND_DIR}/global_theme" "${RELEASE_DIR}/global_theme"
copy_tree "${BACKEND_DIR}/source_root" "${RELEASE_DIR}/source_root"
copy_tree "${BACKEND_DIR}/static_resources" "${RELEASE_DIR}/static_resources"
mkdir -p "${RELEASE_DIR}/build_temp/main_site_out"
mkdir -p "${RELEASE_DIR}/build_temp/book_cache"
mkdir -p "${RELEASE_DIR}/build_temp/full_package"
mkdir -p "${RELEASE_DIR}/publish_records"
write_release_helpers

echo "release package ready: ${RELEASE_DIR}"
