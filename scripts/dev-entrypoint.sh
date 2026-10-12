#!/bin/bash
set -euo pipefail

# Dev sandbox entrypoint — starts kvnode with air fast reload when code exists,
# otherwise sleeps to keep the container alive for shell access.

DEV_DIR="${DEV_DIR:-/workspace}"
SLEEP_ON_IDLE="${SLEEP_ON_IDLE:-true}"

# --- Passwd entry for the runtime user ---------------------------------------
if ! getent passwd "$(id -u)" >/dev/null; then
    if [[ "$(id -u)" -eq 0 ]]; then
        echo "[dev] UID 0 has no passwd entry — adding a fallback identity."
        printf 'sandbox:x:%s:%s:Sandbox User:/home/%s:/bin/bash\n' \
            "$(id -u)" "$(id -g)" "${DEV_USER:-devuser}" >> /etc/passwd
    else
        echo "[dev] WARNING: UID $(id -u) has no passwd entry; tools still work." >&2
    fi
fi

# --- Start kvnode if code exists, else sleep ---------------------------------
if [ "${SLEEP_ON_IDLE}" = "true" ] && { [ ! -f "${DEV_DIR}/go.mod" ] || [ ! -d "${DEV_DIR}/backend/cmd/kvnode" ]; }; then
    echo "[dev] No runnable code yet (go.mod or backend/cmd/kvnode missing); sleeping indefinitely for shell access..."
    exec sleep infinity
fi

echo "[dev] Starting kvnode with air fast reload..."
cd "${DEV_DIR}" && exec air -c .air.toml