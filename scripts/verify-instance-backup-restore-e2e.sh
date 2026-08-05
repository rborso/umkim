#!/usr/bin/env bash
# E2E: whoami-helm smoke, Gitea same-cluster restore, cross-cluster restore (lab-sn01 → lab-ha01).
#
# Required env:
#   FLEET_EDGE_DOMAIN=apps.example.com
# Optional:
#   SKIP_CROSS_CLUSTER=1
#   GITEA_ADMIN_PASS=...
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API="${DCM_API:-http://127.0.0.1:30908/api/v1alpha1}"
DOMAIN="${FLEET_EDGE_DOMAIN:-apps.example.com}"
SOURCE_CLUSTER="${FLEET_BACKUP_SOURCE_CLUSTER:-lab-sn01}"
TARGET_CLUSTER="${FLEET_BACKUP_TARGET_CLUSTER:-lab-ha01}"
WHOAMI_RELEASE="${WHOAMI_RELEASE:-whoami-br-smoke}"
GITEA_RELEASE="${GITEA_RELEASE:-gitea-br-test}"
GITEA_NS="${GITEA_NAMESPACE:-gitea}"
GITEA_PASS="${GITEA_ADMIN_PASS:-FleetGitea-ChangeMe}"
GITEA_USER="${GITEA_ADMIN_USER:-fleet}"
VELERO="${VELERO_CLI:-/usr/local/bin/velero}"
INV="${INV:--i inventory/hosts.yml -i inventory/hosts-from-hub.yml}"
ANSIBLE_EXTRA=()
[[ -n "${FLEET_HUB_IP:-}" ]] && ANSIBLE_EXTRA=(-e "ansible_host=${FLEET_HUB_IP}")
VAULT_ARGS=()
[[ -f "${HOME}/.umkim-vault-pass" ]] && VAULT_ARGS=(--vault-password-file "${HOME}/.umkim-vault-pass")

log() { echo "[e2e] $*"; }
fail() { echo "[e2e] FAIL: $*" >&2; exit 1; }

FLEET_SITE_API="${FLEET_SITE_API:-http://127.0.0.1:30912/fleet/api/v1alpha1}"
FLEET_SITE_COOKIE_JAR="${TMPDIR:-/tmp}/fleet-e2e-site-cookies.txt"

fleet_site_curl() {
  local extra=()
  if [[ -f "$FLEET_SITE_COOKIE_JAR" ]]; then
    extra=(-b "$FLEET_SITE_COOKIE_JAR")
  fi
  curl -sf "${extra[@]}" "$@"
}

ensure_fleet_site_auth() {
  local base="${FLEET_SITE_API%/fleet/api/v1alpha1}"
  local auth="${base}/fleet/auth/v1alpha1"
  local user="${FLEET_AUTH_USER:-localadmin}"
  local pass="${FLEET_AUTH_PASSWORD:-}"
  if [[ -z "$pass" && -f "${HOME}/.umkim-vault-pass" && -f "${ROOT}/inventory/group_vars/all/vault.yml" ]]; then
    pass=$(ansible-vault view "${ROOT}/inventory/group_vars/all/vault.yml" \
      --vault-password-file "${HOME}/.umkim-vault-pass" 2>/dev/null \
      | awk -F'"' '/fleet_console_auth_admin_password_vault/{print $2; exit}')
  fi
  pass="${pass:-fleet-admin-change-me}"
  rm -f "$FLEET_SITE_COOKIE_JAR"
  log "fleet-site login ${user} (console ${base})"
  curl -sf -c "$FLEET_SITE_COOKIE_JAR" -X POST "${auth}/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}" >/dev/null
}

gitea_instance_id() {
  curl -sf "${API}/catalog-item-instances" | jq -r --arg n "$GITEA_RELEASE" --arg c "$SOURCE_CLUSTER" '
    [.results[]
      | select(.display_name == $n or .display_name == ($n + "-cross"))
      | {id: (.path | split("/") | last), cluster: ([.spec.user_values[]? | select(.path == "routing.cluster") | .value] | first // "")}
    ]
    | sort_by(.id)
    | (map(select(.cluster == $c)) + map(select(.cluster != $c)))
    | .[0].id // empty'
}

wait_helm_history() {
  local id="$1"
  for _ in $(seq 1 30); do
    if curl -sf "${API}/catalog-item-instances/${id}/helm/history" | jq -e '.releases | length >= 1' >/dev/null 2>&1; then
      return 0
    fi
    sleep 10
  done
  fail "helm history not ready for ${id}"
}

GATEWAY_HTTP="${HUB_GATEWAY:-http://127.0.0.1:30908}"
GATEWAY_HTTPS="${HUB_GATEWAY_HTTPS:-https://127.0.0.1:30909}"
USE_HTTPS="${FLEET_E2E_USE_HTTPS:-true}"

hub_curl() {
  local host="$1"
  if [[ "$USE_HTTPS" == "true" ]]; then
    curl -sk -H "Host: ${host}" "${GATEWAY_HTTPS}/"
  else
    curl -sfL -H "Host: ${host}" "${GATEWAY_HTTP}/"
  fi
}

hub_http_code() {
  local host="$1"
  if [[ "$USE_HTTPS" == "true" ]]; then
    curl -sk -o /dev/null -w '%{http_code}' -H "Host: ${host}" "${GATEWAY_HTTPS}/" || echo 000
  else
    curl -s -o /dev/null -w '%{http_code}' -H "Host: ${host}" "${GATEWAY_HTTP}/" || echo 000
  fi
}

wait_hub_route() {
  local host="$1"
  local expect_body="${2:-}"
  for _ in $(seq 1 45); do
    local code
    code=$(hub_http_code "$host")
    if [[ "$code" == "200" || "$code" == "302" || "$code" == "401" ]]; then
      if [[ -z "$expect_body" ]] || hub_curl "$host" 2>/dev/null | grep -qi "$expect_body"; then
        return 0
      fi
    fi
    sleep 10
  done
  fail "hub route not ready for ${host}"
}

if [[ "${SKIP_BACKUP_INFRA:-}" == "1" ]]; then
  log "Phase 1: SKIP backup infra (SKIP_BACKUP_INFRA=1)"
else
  log "Phase 1: deploy fleet backup infrastructure"
  ansible-playbook ${INV} fleet-backup.yml -l hub "${ANSIBLE_EXTRA[@]}" --tags fleet_backup_hub,fleet_backup_spokes "${VAULT_ARGS[@]}"
fi

if [[ "${SKIP_CHART_SEED:-}" == "1" ]]; then
  log "Phase 2: SKIP chart push + catalog seed (SKIP_CHART_SEED=1)"
else
  log "Phase 2: push umkim-catalog charts + seed catalog"
  ansible-playbook ${INV} harbor.yml -l hub "${ANSIBLE_EXTRA[@]}" --tags harbor_push_charts "${VAULT_ARGS[@]}"
  "${ROOT}/scripts/seed-umkim-catalog.sh" --seed-only
fi

if [[ "${SKIP_WHOAMI:-}" == "1" ]]; then
  log "Phase 3: SKIP whoami-helm smoke (SKIP_WHOAMI=1)"
else
  log "Phase 3: whoami-helm smoke on ${SOURCE_CLUSTER}"
  ansible-playbook ${INV} playbooks/flows/whoami-helm-deploy.yml \
    -l hub "${ANSIBLE_EXTRA[@]}" \
    -e "fleet_flow_cluster=${SOURCE_CLUSTER}" \
    -e "fleet_flow_release_name=${WHOAMI_RELEASE}" \
    "${VAULT_ARGS[@]}"

  WHOAMI_HOST="${WHOAMI_RELEASE}.${DOMAIN}"
  wait_hub_route "$WHOAMI_HOST" Hostname
  hub_curl "$WHOAMI_HOST" | grep -qi Hostname || fail "whoami smoke body"
  log "whoami-helm OK at ${WHOAMI_HOST}"

  WHOAMI_INST=$(curl -sf "${API}/catalog-item-instances" | jq -r --arg n "$WHOAMI_RELEASE" \
    '.results[] | select(.display_name==$n) | (.path | split("/") | last // .id // .uid)' | head -1)
  [[ -n "$WHOAMI_INST" ]] && curl -sf -X DELETE "${API}/catalog-item-instances/${WHOAMI_INST}" || true
fi

log "Phase 4: deploy Gitea on ${SOURCE_CLUSTER}"
ansible-playbook ${INV} playbooks/flows/gitea-deploy.yml \
  -l hub "${ANSIBLE_EXTRA[@]}" \
  -e @playbooks/flows/examples/gitea-lab-sn01.yml \
  -e "fleet_flow_release_name=${GITEA_RELEASE}" \
  -e "fleet_flow_namespace=${GITEA_NS}" \
  "${VAULT_ARGS[@]}"

sleep 60
GITEA_INST=$(gitea_instance_id)
[[ -n "$GITEA_INST" ]] || fail "gitea instance not found"
wait_helm_history "$GITEA_INST"

GITEA_HOST="${GITEA_RELEASE}.${DOMAIN}"
wait_hub_route "$GITEA_HOST"
export GITEA_MARKER_CONTENT="${GITEA_MARKER_CONTENT:-fleet-backup-marker-e2e}"
"${ROOT}/scripts/verify-gitea-health.sh" --host "$GITEA_HOST" --cluster "$SOURCE_CLUSTER" --admin-user "$GITEA_USER" --admin-pass "$GITEA_PASS" --seed-repo
REPO_SHA=$("${ROOT}/scripts/verify-gitea-health.sh" --host "$GITEA_HOST" --cluster "$SOURCE_CLUSTER" --admin-user "$GITEA_USER" --admin-pass "$GITEA_PASS" 2>&1 \
  | sed -n 's/^backup-test repo sha=//p' | head -1)
[[ -n "$REPO_SHA" ]] || fail "could not read backup-test sha"

log "Phase 5: backup Gitea instance ${GITEA_INST}"
if [[ "${FLEET_USE_BACKUP_API:-}" == "1" ]]; then
  ensure_fleet_site_auth
  log "using fleet-site API via console ${FLEET_SITE_API}"
  BACKUP_JSON=$(fleet_site_curl -X POST "${FLEET_SITE_API}/instance-backups" \
    -H 'Content-Type: application/json' \
    -d "{\"instance_id\":\"${GITEA_INST}\",\"cluster\":\"${SOURCE_CLUSTER}\",\"release\":\"${GITEA_RELEASE}\",\"namespace\":\"${GITEA_NS}\",\"wait\":true}")
  BACKUP_CR=$(echo "$BACKUP_JSON" | jq -r '.name')
  for _ in $(seq 1 60); do
    PHASE=$(fleet_site_curl "${FLEET_SITE_API}/instance-backups/${BACKUP_CR}" | jq -r '.phase')
    [[ "$PHASE" == "Complete" ]] && break
    [[ "$PHASE" == "Failed" ]] && fail "API backup failed"
    sleep 10
  done
  BACKUP_NAME=$(fleet_site_curl "${FLEET_SITE_API}/instance-backups/${BACKUP_CR}" | jq -r '.velero_backup_name')
  log "API backup ${BACKUP_CR} velero=${BACKUP_NAME}"
else
  BACKUP_DIR=$("${ROOT}/scripts/backup-spoke-instance.sh" \
    --cluster "$SOURCE_CLUSTER" \
    --instance-id "$GITEA_INST" \
    --release "$GITEA_RELEASE" \
    --namespace "$GITEA_NS" \
    --wait)
  BACKUP_NAME=$(cat "${BACKUP_DIR}/velero-backup-name.txt")
  BACKUP_CR=""
  log "backup ${BACKUP_NAME} at ${BACKUP_DIR}"
fi

log "Phase 6: simulate disaster (delete DCM instance)"
curl -sf -X DELETE "${API}/catalog-item-instances/${GITEA_INST}"
sleep 30

log "Phase 7: same-cluster restore on ${SOURCE_CLUSTER}"
if [[ "${FLEET_USE_BACKUP_API:-}" == "1" ]]; then
  RESTORE_JSON=$(fleet_site_curl -X POST "${FLEET_SITE_API}/instance-restores" \
    -H 'Content-Type: application/json' \
    -d "{\"backup_name\":\"${BACKUP_CR}\",\"target_cluster\":\"${SOURCE_CLUSTER}\",\"rebind\":{\"enabled\":true,\"mode\":\"supersede\"}}")
  RESTORE_CR=$(echo "$RESTORE_JSON" | jq -r '.name')
  for _ in $(seq 1 90); do
    PHASE=$(fleet_site_curl "${FLEET_SITE_API}/instance-restores/${RESTORE_CR}" | jq -r '.phase')
    [[ "$PHASE" == "Complete" ]] && break
    [[ "$PHASE" == "Failed" ]] && fail "API restore failed"
    sleep 10
  done
  NEW_INST=$(fleet_site_curl "${FLEET_SITE_API}/instance-restores/${RESTORE_CR}" | jq -r '.new_instance_id')
else
  "${ROOT}/scripts/restore-spoke-instance.sh" \
    --cluster "$SOURCE_CLUSTER" \
    --backup "$BACKUP_NAME" \
    --namespace "$GITEA_NS" \
    --release "$GITEA_RELEASE"

  NEW_INST=$("${ROOT}/scripts/rebind-dcm-instance.sh" \
    --manifest "${BACKUP_DIR}/dcm-instance.json" \
    --cluster "$SOURCE_CLUSTER" \
    --mode supersede \
    --source-instance-id "$GITEA_INST")
fi
log "rebound instance ${NEW_INST}"
sleep 45
wait_hub_route "$GITEA_HOST"
"${ROOT}/scripts/verify-gitea-health.sh" --host "$GITEA_HOST" --cluster "$SOURCE_CLUSTER" --admin-user "$GITEA_USER" --admin-pass "$GITEA_PASS" --expect-sha "$REPO_SHA"

log "Phase 8: cross-cluster restore ${SOURCE_CLUSTER} → ${TARGET_CLUSTER}"
if [[ "${SKIP_CROSS_CLUSTER:-}" == "1" ]]; then
  log "SKIP cross-cluster (SKIP_CROSS_CLUSTER=1)"
else
  SC_MAP="${FLEET_VELERO_SC_MAP:-local-path:longhorn}"
  if [[ "${FLEET_USE_BACKUP_API:-}" == "1" ]]; then
    RESTORE_JSON=$(fleet_site_curl -X POST "${FLEET_SITE_API}/instance-restores" \
      -H 'Content-Type: application/json' \
      -d "{
        \"backup_name\": \"${BACKUP_CR}\",
        \"target_cluster\": \"${TARGET_CLUSTER}\",
        \"source_cluster\": \"${SOURCE_CLUSTER}\",
        \"rebind\": {
          \"enabled\": true,
          \"mode\": \"supersede\",
          \"user_value_patches\": [{\"path\": \"routing.cluster\", \"value\": \"${TARGET_CLUSTER}\"}]
        },
        \"storage_class_hints\": {\"local-path\": \"longhorn\"}
      }")
    CROSS_RESTORE=$(echo "$RESTORE_JSON" | jq -r '.name')
    for _ in $(seq 1 90); do
      PHASE=$(fleet_site_curl "${FLEET_SITE_API}/instance-restores/${CROSS_RESTORE}" | jq -r '.phase')
      [[ "$PHASE" == "Complete" ]] && break
      [[ "$PHASE" == "Failed" ]] && fail "API cross-cluster restore failed"
      sleep 10
    done
    CROSS_INST=$(fleet_site_curl "${FLEET_SITE_API}/instance-restores/${CROSS_RESTORE}" | jq -r '.new_instance_id')
  else
    "${ROOT}/scripts/restore-spoke-instance.sh" \
      --cluster "$TARGET_CLUSTER" \
      --backup "$BACKUP_NAME" \
      --source-cluster "$SOURCE_CLUSTER" \
      --namespace "$GITEA_NS" \
      --release "$GITEA_RELEASE" \
      --storage-class-map "$SC_MAP"

    CROSS_INST=$("${ROOT}/scripts/rebind-dcm-instance.sh" \
      --manifest "${BACKUP_DIR}/dcm-instance.json" \
      --cluster "$TARGET_CLUSTER" \
      --mode supersede \
      --source-instance-id "$NEW_INST" \
      --patch "routing.cluster=${TARGET_CLUSTER}")
  fi
  log "cross-cluster instance ${CROSS_INST}"
  DUP_COUNT=$(curl -sf "${API}/catalog-item-instances" | jq -r --arg n "$GITEA_RELEASE" \
    '[.results[] | select(.display_name == $n)] | length')
  [[ "$DUP_COUNT" -le 1 ]] || log "WARN: expected one ${GITEA_RELEASE} instance after supersede, found ${DUP_COUNT}"
  sleep 60
  wait_hub_route "$GITEA_HOST"
  "${ROOT}/scripts/verify-gitea-health.sh" --host "$GITEA_HOST" --cluster "$TARGET_CLUSTER" --admin-user "$GITEA_USER" --admin-pass "$GITEA_PASS" --expect-sha "$REPO_SHA"

  PLACEMENT=$(curl -sf "${API}/catalog-item-instances/${CROSS_INST}/placement-targets" || echo '{}')
  echo "$PLACEMENT" | jq -e --arg c "$TARGET_CLUSTER" '.targets[]? | select(.cluster==$c)' >/dev/null \
    || log "WARN: placement targets may still be settling"
fi

log "=== verify-instance-backup-restore-e2e OK ==="
