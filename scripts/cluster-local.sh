#!/usr/bin/env bash
set -euo pipefail

# cluster-local.sh — run a persistent kvnode cluster with local processes (no
# docker), so the dashboard can show followers and leader failover. Three nodes
# by default; set NODE_COUNT for a different size. Unlike cluster-smoke.sh — a
# one-shot pass/fail check that tears its nodes down on exit — these nodes stay
# up until `cluster-local-down`.
#
# Usage:
#   scripts/cluster-local.sh up      start the cluster (leaves it running)
#   scripts/cluster-local.sh down    stop the cluster
#   scripts/cluster-local.sh status  print each node's raft role
#
# State (pids, logs, data, built binary) lives under .cluster/ (gitignored).
# Each `up` starts a fresh cluster — any previous data is cleared.
#
# Environment overrides:
#   NODE_COUNT  number of nodes to start (default 3); any size ≥ 1 is allowed —
#               quorum is ⌊N/2⌋ + 1, so even sizes are safe (e.g. NODE_COUNT=4
#               needs 3 of 4 to commit).
#   BASE_PORT   starting port (default 9081); node i listens on BASE_PORT+i-1.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BASE_PORT="${BASE_PORT:-9081}"
NODE_COUNT="${NODE_COUNT:-3}"

STATE_DIR="$ROOT/.cluster"
BIN="$STATE_DIR/bin/kvnode"
PIDFILE="$STATE_DIR/pids"
LOGDIR="$STATE_DIR/logs"
DATADIR="$STATE_DIR/data"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

# NODE_COUNT must be a positive integer before it feeds any arithmetic.
case "$NODE_COUNT" in
  '' | *[!0-9]*) die "NODE_COUNT must be a positive integer, got '$NODE_COUNT'" ;;
esac
[ "$NODE_COUNT" -ge 1 ] || die "NODE_COUNT must be at least 1, got '$NODE_COUNT'"

node_ids() {
  local i
  for ((i = 0; i < NODE_COUNT; i++)); do printf 'node%d\n' "$((i + 1))"; done
}

node_index() { # id -> zero-based index (node1 -> 0)
  case "$1" in
    node*) printf '%d' "$(( ${1#node} - 1 ))" ;;
    *) die "unexpected node id: $1" ;;
  esac
}

node_port() { printf '%d' "$((BASE_PORT + $(node_index "$1")))"; }
node_url() { printf 'http://127.0.0.1:%s' "$(node_port "$1")"; }

# peers_for prints "node2=http://...,node3=http://..." for the given node.
peers_for() {
  local self="$1" id out=""
  for id in $(node_ids); do
    [ "$id" = "$self" ] && continue
    [ -n "$out" ] && out="$out,"
    out="$out$id=$(node_url "$id")"
  done
  printf '%s' "$out"
}

# alive reports whether pid is a live (non-zombie) process. A zombie has
# already exited but keeps a slot in the process table until reaped, and
# `kill -0` still succeeds for it — so check the process state too.
alive() {
  local pid="${1:-}" stat
  [ -n "$pid" ] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  stat="$(ps -o stat= -p "$pid" 2>/dev/null || true)"
  [ -n "$stat" ] || return 1
  case "$stat" in Z*) return 1 ;; esac
  return 0
}

port_in_use() {
  local port="$1"
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    return 0
  fi
  return 1
}

# role_of prints the node's raft role, or nothing if it cannot be reached.
role_of() {
  curl -fsS --max-time 1 "$1/v1/status" 2>/dev/null \
    | sed -n 's/.*"role":"\([^"]*\)".*/\1/p' || true
}

pid_of() { awk -v id="$1" '$1 == id { print $2 }' "$PIDFILE"; }

cmd_up() {
  if [ -f "$PIDFILE" ]; then
    local id pid
    while read -r id pid; do
      [ -n "${id:-}" ] || continue
      alive "${pid:-}" && die "cluster already running ($id pid $pid) — run 'make cluster-local-down' first"
    done < "$PIDFILE"
    rm -f "$PIDFILE" # stale file from a crashed run
  fi

  local id
  for id in $(node_ids); do
    port_in_use "$(node_port "$id")" \
      && die "port $(node_port "$id") already in use (set BASE_PORT=... to move the cluster)"
  done

  mkdir -p "$(dirname "$BIN")" "$LOGDIR"
  # Start fresh each time so a re-run never inherits a previous cluster's data.
  rm -rf "$DATADIR"
  mkdir -p "$DATADIR"

  log "building kvnode"
  go build -o "$BIN" ./backend/cmd/kvnode

  : > "$PIDFILE"
  for id in $(node_ids); do
    local port peers
    port="$(node_port "$id")"
    peers="$(peers_for "$id")"
    log "starting $id on 127.0.0.1:$port (peers: $peers)"
    "$BIN" -id "$id" -http "127.0.0.1:$port" -peers "$peers" \
      -data-dir "$DATADIR/$id" >"$LOGDIR/$id.log" 2>&1 &
    echo "$id $!" >> "$PIDFILE"
  done

  log "waiting for a leader"
  local deadline=$((SECONDS + 20)) role
  while (( SECONDS < deadline )); do
    for id in $(node_ids); do
      alive "$(pid_of "$id")" || die "node $id exited during startup — see $LOGDIR/$id.log"
      role="$(role_of "$(node_url "$id")")"
      if [ "$role" = "leader" ]; then
        log "leader is $id"
        print_urls
        return 0
      fi
    done
    sleep 0.2
  done
  die "no leader elected within 20s — see $LOGDIR/*.log"
}

print_urls() {
  local id role
  printf '\nCluster is up — open any node dashboard:\n'
  for id in $(node_ids); do
    role="$(role_of "$(node_url "$id")")"
    printf '  %-6s %s/  (%s)\n' "$id" "$(node_url "$id")" "${role:-unreachable}"
  done
  printf '\nStop it with: make cluster-local-down\n'
}

cmd_down() {
  if [ ! -f "$PIDFILE" ]; then
    log "no cluster running (no $PIDFILE)"
    return 0
  fi

  local id pid
  while read -r id pid; do
    [ -n "${id:-}" ] || continue
    if alive "${pid:-}"; then
      log "stopping $id (pid $pid)"
      kill "${pid:-}" 2>/dev/null || true
    fi
  done < "$PIDFILE"

  local deadline=$((SECONDS + 5)) any
  while (( SECONDS < deadline )); do
    any=0
    while read -r id pid; do
      [ -n "${id:-}" ] || continue
      alive "${pid:-}" && any=1
    done < "$PIDFILE"
    [ "$any" -eq 0 ] && break
    sleep 0.2
  done

  while read -r id pid; do
    [ -n "${id:-}" ] || continue
    if alive "${pid:-}"; then
      log "force-stopping $id"
      kill -9 "${pid:-}" 2>/dev/null || true
    fi
  done < "$PIDFILE"

  rm -f "$PIDFILE"
  log "cluster stopped"
}

cmd_status() {
  if [ ! -f "$PIDFILE" ]; then
    die "cluster not running (no $PIDFILE) — start it with 'make cluster-local-up'"
  fi
  local id pid role
  while read -r id pid; do
    [ -n "${id:-}" ] || continue
    if alive "${pid:-}"; then
      role="$(role_of "$(node_url "$id")")"
      printf '%-6s pid %-6s role %s\n' "$id" "$pid" "${role:-unreachable}"
    else
      printf '%-6s (not running)\n' "$id"
    fi
  done < "$PIDFILE"
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  status) cmd_status ;;
  *) die "usage: $(basename "$0") {up|down|status}" ;;
esac
