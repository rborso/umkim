#!/usr/bin/env bash
# Smoke-test fleet-site instance backup/restore HTTP API.
#
# Usage:
#   ./scripts/verify-instance-backup-api.sh --instance-id <uuid>
# Env:
#   FLEET_SITE_API       default http://127.0.0.1:30912/fleet/api/v1alpha1 (via console; needs auth)
#   FLEET_SITE_API_DIRECT  hub-local bypass, e.g. http://127.0.0.1:18080/api/v1alpha1
#   FLEET_AUTH_USER / FLEET_AUTH_PASSWORD  for console NodePort (when not using DIRECT)
set -euo pipefail

API="${FLEET_SITE_API_DIRECT:-${FLEET_SITE_API:-http://127.0.0.1:30912/fleet/api/v1alpha1}}"
INSTANCE_ID=""
COOKIE_JAR="${TMPDIR:-/tmp}/fleet-backup-api-cookies.txt"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --instance-id) INSTANCE_ID="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,16p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$INSTANCE_ID" ]] || { echo "ERROR: --instance-id required" >&2; exit 1; }

log() { echo "[backup-api] $*"; }
fail() { echo "[backup-api] FAIL: $*" >&2; exit 1; }

api_curl() {
  local extra=()
  if [[ -z "${FLEET_SITE_API_DIRECT:-}" && -f "$COOKIE_JAR" ]]; then
    extra=(-b "$COOKIE_JAR")
  fi
  curl -sf "${extra[@]}" "$@"
}

if [[ -z "${FLEET_SITE_API_DIRECT:-}" ]]; then
  BASE="${FLEET_SITE_API%/fleet/api/v1alpha1}"
  AUTH="${BASE}/fleet/auth/v1alpha1"
  USER="${FLEET_AUTH_USER:-localadmin}"
  PASS="${FLEET_AUTH_PASSWORD:-fleet-admin-change-me}"
  rm -f "$COOKIE_JAR"
  log "login ${USER}@${BASE}"
  curl -sf -c "$COOKIE_JAR" -X POST "${AUTH}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${USER}\",\"password\":\"${PASS}\"}" >/dev/null
fi

poll_phase() {
  local kind="$1" name="$2" want="$3"
  for _ in $(seq 1 90); do
    local phase
    phase=$(api_curl "${API}/${kind}/${name}" | jq -r '.phase // empty')
    log "${kind} ${name} phase=${phase}"
    if [[ "$phase" == "$want" ]]; then
      return 0
    fi
    if [[ "$phase" == "Failed" ]]; then
      api_curl "${API}/${kind}/${name}" | jq .
      fail "${kind} ${name} failed"
    fi
    sleep 10
  done
  fail "${kind} ${name} timed out waiting for ${want}"
}

log "GET /backup-infrastructure"
api_curl "${API}/backup-infrastructure" | jq .

log "GET /instance-workloads/${INSTANCE_ID}"
WORKLOAD=$(api_curl "${API}/instance-workloads/${INSTANCE_ID}")
echo "$WORKLOAD" | jq .
CLUSTER=$(echo "$WORKLOAD" | jq -r '.cluster // empty')
RELEASE=$(echo "$WORKLOAD" | jq -r '.release // empty')
[[ -n "$CLUSTER" && -n "$RELEASE" ]] || fail "workload not resolved"

log "POST /instance-backups"
BACKUP=$(api_curl -X POST "${API}/instance-backups" \
  -H 'Content-Type: application/json' \
  -d "{\"instance_id\":\"${INSTANCE_ID}\",\"wait\":true}")
echo "$BACKUP" | jq .
BACKUP_NAME=$(echo "$BACKUP" | jq -r '.name // empty')
[[ -n "$BACKUP_NAME" ]] || fail "backup not accepted"
poll_phase "instance-backups" "$BACKUP_NAME" "Complete"

log "POST /instance-restores (same cluster, rebind supersede)"
RESTORE=$(api_curl -X POST "${API}/instance-restores" \
  -H 'Content-Type: application/json' \
  -d "{
    \"backup_name\": \"${BACKUP_NAME}\",
    \"target_cluster\": \"${CLUSTER}\",
    \"rebind\": { \"enabled\": true, \"mode\": \"supersede\" }
  }")
echo "$RESTORE" | jq .
RESTORE_NAME=$(echo "$RESTORE" | jq -r '.name // empty')
[[ -n "$RESTORE_NAME" ]] || fail "restore not accepted"
poll_phase "instance-restores" "$RESTORE_NAME" "Complete"

log "=== verify-instance-backup-api OK ==="
