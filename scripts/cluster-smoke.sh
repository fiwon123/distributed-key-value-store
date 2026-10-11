#!/usr/bin/env bash
set -euo pipefail

# cluster-smoke.sh — the project's "Done when", as an executable check:
#
#   1. start three real kvnode processes with persistent data dirs
#   2. write a key to the leader
#   3. kill the leader
#   4. wait for a new leader and read the key back
#
# Runs on any host with Go + curl — no docker required. For the
# containerized cluster use `make cluster-up` (deploy/docker-compose.yml).
#
# Override the starting port with BASE_PORT (default 9081).

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BASE_PORT="${BASE_PORT:-9081}"
NODE_COUNT=3
NODE_IDS=(node1 node2 node3)
KEY="smoke"
VALUE="committed-$$"

WORK="$(mktemp -d)"
PORTS=()
URLS=()
PIDS=()

cleanup() {
  local pid
  for pid in "${PIDS[@]:-}"; do
    [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

log() { printf '==> %s\n' "$*"; }

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  for id in "${NODE_IDS[@]}"; do
    [ -f "$WORK/$id.log" ] && { printf -- '--- %s.log ---\n' "$id" >&2; tail -n 20 "$WORK/$id.log" >&2; }
  done
  exit 1
}

# alive reports whether pid is still running.
alive() { kill -0 "$1" 2>/dev/null; }

# role_of returns the node's raft role, or empty if it cannot be reached.
role_of() {
  curl -fsS --max-time 1 "$1/v1/status" 2>/dev/null \
    | sed -n 's/.*"role":"\([^"]*\)".*/\1/p'
}

# wait_for_leader polls the live nodes for a leader. Sets LEADER_IDX.
wait_for_leader() {
  local deadline=$((SECONDS + 20)) i role
  while (( SECONDS < deadline )); do
    for i in "${!PORTS[@]}"; do
      [ -n "${PIDS[$i]:-}" ] || continue
      alive "${PIDS[$i]}" || continue
      role="$(role_of "${URLS[$i]}")"
      if [ "$role" = "leader" ]; then
        LEADER_IDX=$i
        return 0
      fi
    done
    sleep 0.2
  done
  return 1
}

log "building kvnode"
go build -o "$WORK/kvnode" ./cmd/kvnode

# Start each node with its peers; node i's peers are all other nodes.
for i in "${!NODE_IDS[@]}"; do
  port=$((BASE_PORT + i))
  PORTS[$i]=$port
  URLS[$i]="http://127.0.0.1:$port"

  peers=""
  for j in "${!NODE_IDS[@]}"; do
    [ "$j" -eq "$i" ] && continue
    [ -n "$peers" ] && peers="$peers,"
    peers="$peers${NODE_IDS[$j]}=http://127.0.0.1:$((BASE_PORT + j))"
  done

  id="${NODE_IDS[$i]}"
  log "starting $id on :$port (peers: $peers)"
  "$WORK/kvnode" -id "$id" -http "127.0.0.1:$port" \
    -peers "$peers" -data-dir "$WORK/data/$id" \
    >"$WORK/$id.log" 2>&1 &
  PIDS[$i]=$!
done

log "waiting for an initial leader"
wait_for_leader || fail "no leader elected within 20s"
OLD=$LEADER_IDX
log "leader is ${NODE_IDS[$OLD]} (${URLS[$OLD]})"

log "PUT $KEY=$VALUE to the leader"
curl -fsS -X PUT --data-binary "$VALUE" "${URLS[$OLD]}/v1/kv/$KEY" >/dev/null \
  || fail "PUT to leader failed"

log "killing leader ${NODE_IDS[$OLD]} (pid ${PIDS[$OLD]})"
kill "${PIDS[$OLD]}"
wait "${PIDS[$OLD]}" 2>/dev/null || true

log "waiting for a new leader"
wait_for_leader || fail "no new leader elected within 20s"
NEW=$LEADER_IDX
if [ "$NEW" -eq "$OLD" ]; then
  fail "leader index unchanged after kill"
fi
log "new leader is ${NODE_IDS[$NEW]} (${URLS[$NEW]})"

log "GET $KEY from the new leader"
body="$(curl -fsS --max-time 2 "${URLS[$NEW]}/v1/kv/$KEY")" \
  || fail "GET from new leader failed"
case "$body" in
  *"\"value\":\"$VALUE\""*) : ;;
  *) fail "expected value $VALUE, got: $body" ;;
esac

log "PASS: $KEY=$VALUE survived a leader failure"
