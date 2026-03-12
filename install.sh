#!/bin/sh
set -eu

OWNER="shunsukeaihara"
REPO="ComposePilot"
APP="composepilot"
SERVICE_USER="composepilot"

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

if [ "$(id -u)" -ne 0 ]; then
  fail "run as root. Example: curl -fsSL https://raw.githubusercontent.com/${OWNER}/${REPO}/main/install.sh | sudo sh"
fi

need_cmd uname
need_cmd curl
need_cmd tar
need_cmd install

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_RAW="$(uname -m)"

case "$OS" in
  linux)
    ;;
  *)
    fail "unsupported OS: $OS (install.sh currently supports Linux only)"
    ;;
esac

case "$ARCH_RAW" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    fail "unsupported architecture: $ARCH_RAW"
    ;;
esac

VERSION="${COMPOSEPILOT_VERSION:-}"
if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${OWNER}/${REPO}/releases/latest" | sed 's#.*/##')"
fi
[ -n "$VERSION" ] || fail "could not determine release version"

VERSION_NO_V="${VERSION#v}"
ASSET="${APP}_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${ASSET}"

BIN_DIR="${COMPOSEPILOT_BIN_DIR:-/usr/local/bin}"
CONFIG_DIR="${COMPOSEPILOT_CONFIG_DIR:-/etc/composepilot}"
DATA_DIR="${COMPOSEPILOT_DATA_DIR:-/var/lib/composepilot}"
WORKSPACE_DIR="${COMPOSEPILOT_WORKSPACE_DIR:-${DATA_DIR}/workspace}"
ENV_FILE="${CONFIG_DIR}/composepilot.env"
MASTER_KEY_FILE="${CONFIG_DIR}/master_key"
SERVICE_FILE="/etc/systemd/system/composepilot.service"

LISTEN_ADDR="${COMPOSEPILOT_LISTEN:-:8080}"
BIN_PATH="${BIN_DIR}/${APP}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

log "Downloading ${DOWNLOAD_URL}"
curl -fsSL "${DOWNLOAD_URL}" -o "${tmpdir}/${ASSET}"
tar -xzf "${tmpdir}/${ASSET}" -C "$tmpdir"

install -d -m 755 "$BIN_DIR"
install -m 755 "${tmpdir}/${APP}-${OS}-${ARCH}" "$BIN_PATH"

install -d -m 700 "$CONFIG_DIR"
install -d -m 755 "$DATA_DIR" "$WORKSPACE_DIR"

generate_master_key() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32
  elif command -v base64 >/dev/null 2>&1; then
    dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64
  else
    fail "openssl or base64 is required to generate a master key"
  fi
}

if [ ! -f "$MASTER_KEY_FILE" ]; then
  umask 077
  generate_master_key > "$MASTER_KEY_FILE"
  log "Generated ${MASTER_KEY_FILE}"
fi
chmod 600 "$MASTER_KEY_FILE"

if [ ! -f "$ENV_FILE" ]; then
  umask 077
  cat > "$ENV_FILE" <<EOF
COMPOSEPILOT_MASTER_KEY_FILE=${MASTER_KEY_FILE}
EOF
  log "Created ${ENV_FILE}"
fi
chmod 600 "$ENV_FILE"

write_linux_service() {
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=ComposePilot
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${DATA_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN_PATH} -listen ${LISTEN_ADDR} -data-dir ${DATA_DIR} -workspace ${WORKSPACE_DIR}
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
}

ensure_linux_user() {
  if id "${SERVICE_USER}" >/dev/null 2>&1; then
    return
  fi
  if command -v useradd >/dev/null 2>&1; then
    shell_path="$(command -v nologin 2>/dev/null || printf '%s' /usr/sbin/nologin)"
    useradd --system --home-dir "$DATA_DIR" --create-home --shell "$shell_path" "${SERVICE_USER}"
    return
  fi
  if command -v adduser >/dev/null 2>&1; then
    if adduser --help 2>&1 | grep -q -- '--system'; then
      adduser --system --home "$DATA_DIR" --disabled-login "${SERVICE_USER}"
    else
      adduser -S -D -H "${SERVICE_USER}"
    fi
    return
  fi
  fail "could not create ${SERVICE_USER}; neither useradd nor adduser is available"
}

ensure_docker_group_membership() {
  if command -v getent >/dev/null 2>&1; then
    if ! getent group docker >/dev/null 2>&1; then
      log "docker group not found; skipping group membership setup."
      return
    fi
  elif ! grep -q '^docker:' /etc/group 2>/dev/null; then
    log "docker group not found; skipping group membership setup."
    return
  fi

  if command -v usermod >/dev/null 2>&1; then
    usermod -aG docker "${SERVICE_USER}"
    return
  fi
  if command -v adduser >/dev/null 2>&1; then
    adduser "${SERVICE_USER}" docker
    return
  fi
  log "could not add ${SERVICE_USER} to docker group automatically."
}

need_cmd systemctl
ensure_linux_user
ensure_docker_group_membership
chown -R "${SERVICE_USER}:${SERVICE_USER}" "$DATA_DIR"
write_linux_service
chmod 644 "$SERVICE_FILE"
systemctl daemon-reload
systemctl enable --now composepilot
log "ComposePilot installed and started with systemd."

log "Version: ${VERSION}"
log "Binary: ${BIN_PATH}"
log "Config: ${ENV_FILE}"
log "Master key: ${MASTER_KEY_FILE}"
log "Listen address: ${LISTEN_ADDR}"
