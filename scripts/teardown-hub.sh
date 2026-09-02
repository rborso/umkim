#!/usr/bin/env bash
# Complete UMKIM management-hub teardown for a greenfield reinstall.
#
# Destroys all console-provisioned clusters (K3s reset on nodes), removes fleet
# workloads, Harbor, OCM, hub K3s, and WireGuard. After success, run:
#   ./scripts/install-hub.sh
#
# Run ON the hub (or via Ansible with hosts-from-hub.yml). Requires inventory
# from examples/inventory/hub-only (see scripts/bootstrap-hub-inventory.sh).
#
# Usage:
#   ./scripts/teardown-hub.sh --confirm          # required safety flag
#   ./scripts/teardown-hub.sh --confirm --dry-run
#   ./scripts/teardown-hub.sh --confirm --skip-cluster-delete   # hub-only wipe
#
# Environment (same as install-hub.sh):
#   INV, ANSIBLE_VAULT_PASSWORD_FILE
#   FLEET_API              fleet-site API (default http://127.0.0.1:18080/api/v1alpha1)
#   TEARDOWN_CLUSTER_TIMEOUT_SEC  per-cluster delete wait (default 3600)
#
# Options:
#   --confirm              Required. Without this flag the script exits immediately.
#   --dry-run              Print actions; skip API deletes and Ansible changes.
#   --skip-cluster-delete  Skip FCJ / nested cluster API teardown (hub wipe only).
#   --keep-wireguard       Leave wg0 up (not recommended for full reinstall).
#   --keep-host-paths      Skip /var/lib/umkim, /opt/fleet-src cleanup in Ansible.
#   --keep-crds            Skip fleet CRD deletion in Ansible.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export INV="${INV:--i inventory/hosts.yml -i inventory/hosts-from-hub.yml}"

API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"
CLUSTER_TIMEOUT="${TEARDOWN_CLUSTER_TIMEOUT_SEC:-3600}"
POLL_INTERVAL="${TEARDOWN_POLL_INTERVAL_SEC:-20}"
# Direct fleet-site API requires identity headers (FLEET_SITE_REQUIRE_IDENTITY=true).
API_ID_HEADERS=(
  -H "X-Fleet-Subject-Kind: User"
  -H "X-Fleet-Subject-Name: fleet-teardown"
  -H "X-Fleet-DC-Admin: true"
)

CONFIRM=0
DRY_RUN=0
SKIP_CLUSTER_DELETE=0
KEEP_WIREGUARD=0
KEEP_HOST_PATHS=0
KEEP_CRDS=0

usage() {
  sed -n '2,28p' "$0" | sed 's/^# \{0,1\}//'
}

log() { echo "[teardown-hub] $*"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --confirm) CONFIRM=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --skip-cluster-delete) SKIP_CLUSTER_DELETE=1; shift ;;
    --keep-wireguard) KEEP_WIREGUARD=1; shift ;;
    --keep-host-paths) KEEP_HOST_PATHS=1; shift ;;
    --keep-crds) KEEP_CRDS=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ "${CONFIRM}" -ne 1 ]]; then
  echo "ERROR: Refusing to run without --confirm (this destroys the hub and all fleet clusters)." >&2
  usage
  exit 1
fi

VAULT_ARGS=()
if [[ -z "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${HOME}/.umkim-vault-pass" ]]; then
  export ANSIBLE_VAULT_PASSWORD_FILE="${HOME}/.umkim-vault-pass"
fi
if [[ -n "${ANSIBLE_VAULT_PASSWORD_FILE:-}" && -f "${ANSIBLE_VAULT_PASSWORD_FILE}" ]]; then
  VAULT_ARGS=(--vault-password-file "$ANSIBLE_VAULT_PASSWORD_FILE")
else
  echo "ERROR: hub teardown requires encrypted vault.yml and ANSIBLE_VAULT_PASSWORD_FILE (see docs/install.md)." >&2
  exit 1
fi

kubectl_cmd() {
  if command -v kubectl >/dev/null 2>&1 && kubectl cluster-info >/dev/null 2>&1; then
    kubectl "$@"
  else
    sudo kubectl "$@"
  fi
}

api_delete_cluster() {
  local name="$1"
  local url="${API}/clusters/${name}?cleanupKubernetes=true"
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    log "[dry-run] DELETE ${url}"
    return 0
  fi
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${API_ID_HEADERS[@]}" "${url}" || true)
  case "${code}" in
    202|204|404) log "DELETE cluster ${name} → HTTP ${code}" ;;
    409)
      log "Cluster ${name} busy (409); waiting before retry..."
      sleep 30
      code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${API_ID_HEADERS[@]}" "${url}" || true)
      log "DELETE cluster ${name} (retry) → HTTP ${code}"
      ;;
    *)
      log "WARN: DELETE cluster ${name} → HTTP ${code}"
      ;;
  esac
}

wait_cluster_gone() {
  local name="$1"
  local max="${CLUSTER_TIMEOUT}"
  local elapsed=0
  while [[ "${elapsed}" -lt "${max}" ]]; do
    local code
    code=$(curl -s -o /dev/null -w '%{http_code}' "${API}/clusters/${name}" || true)
    if [[ "${code}" == "404" ]]; then
      log "Cluster ${name} removed from fleet API"
      return 0
    fi
    sleep "${POLL_INTERVAL}"
    elapsed=$((elapsed + POLL_INTERVAL))
  done
  echo "ERROR: timeout waiting for cluster ${name} to disappear (${max}s)" >&2
  return 1
}

wait_delete_jobs() {
  local max="${CLUSTER_TIMEOUT}"
  local elapsed=0
  while [[ "${elapsed}" -lt "${max}" ]]; do
    local pending
    pending=$(kubectl_cmd get jobs -n fleet-console -o name 2>/dev/null \
      | grep -E 'fleet-cluster-(delete|provider)' || true)
    if [[ -z "${pending}" ]]; then
      log "No in-flight fleet cluster delete/provider jobs"
      return 0
    fi
    log "Waiting for jobs: ${pending//$'\n'/; }"
    sleep "${POLL_INTERVAL}"
    elapsed=$((elapsed + POLL_INTERVAL))
  done
  log "WARN: timed out waiting for fleet-console jobs (continuing hub teardown)"
}

list_fleet_clusters() {
  curl -sf "${API}/clusters" 2>/dev/null | python3 -c "
import json, sys
data = json.load(sys.stdin)
for c in data.get('clusters', []):
    print(c['name'])
" 2>/dev/null || true
}

list_nested_clusters() {
  curl -sf "${API}/nested-clusters" 2>/dev/null | python3 -c "
import json, sys
data = json.load(sys.stdin)
for c in data.get('items', data.get('nested_clusters', data.get('clusters', []))):
    if isinstance(c, dict):
        print(c.get('name', c.get('metadata', {}).get('name', '')))
    else:
        print(c)
" 2>/dev/null || true
}

api_delete_nested() {
  local name="$1"
  local url="${API}/nested-clusters/${name}"
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    log "[dry-run] DELETE ${url}"
    return 0
  fi
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${API_ID_HEADERS[@]}" "${url}" || true)
  log "DELETE nested cluster ${name} → HTTP ${code}"
}

kubectl_fallback_cluster_cleanup() {
  log "Fleet API unavailable or cluster delete incomplete — kubectl fallback for FCJ/ManagedCluster"
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    log "[dry-run] kubectl delete fleetclusterjoin --all -n fleet-console"
    log "[dry-run] kubectl delete managedcluster --all"
    return 0
  fi
  kubectl_cmd delete fleetclusterjoin --all -n fleet-console --ignore-not-found --wait=false 2>/dev/null || true
  kubectl_cmd delete managedcluster --all --ignore-not-found --wait=false 2>/dev/null || true
}

echo "=== UMKIM hub teardown ==="
echo "INV=${INV} API=${API}"
if [[ "${DRY_RUN}" -eq 1 ]]; then
  echo "DRY RUN — no destructive changes"
fi
echo ""
echo "This will:"
echo "  1. Delete ALL fleet clusters with cleanupKubernetes=true (uninstall K3s on nodes)"
echo "  2. Delete nested clusters (if any)"
echo "  3. Remove fleet edge, Harbor, OCM, hub ingress, and hub K3s"
if [[ "${KEEP_WIREGUARD}" -eq 0 ]]; then
  echo "  4. Stop hub WireGuard (wg0) and drop nft table fleet_hub"
fi
echo ""
sleep 3

# --- Phase 1: workload clusters (FCJ) ---
if [[ "${SKIP_CLUSTER_DELETE}" -eq 0 ]]; then
  log "Phase 1: delete fleet clusters (K3s reset on nodes)"
  mapfile -t CLUSTERS < <(list_fleet_clusters)
  if [[ "${#CLUSTERS[@]}" -eq 0 ]]; then
    log "No clusters reported by fleet API"
  else
    log "Deleting ${#CLUSTERS[@]} cluster(s): ${CLUSTERS[*]}"
    for name in "${CLUSTERS[@]}"; do
      [[ -n "${name}" ]] || continue
      api_delete_cluster "${name}"
    done
    for name in "${CLUSTERS[@]}"; do
      [[ -n "${name}" ]] || continue
      if [[ "${DRY_RUN}" -eq 0 ]]; then
        wait_cluster_gone "${name}" || kubectl_fallback_cluster_cleanup
      fi
    done
    if [[ "${DRY_RUN}" -eq 0 ]]; then
      wait_delete_jobs || true
    fi
  fi

  log "Phase 2: delete nested clusters"
  mapfile -t NESTED < <(list_nested_clusters)
  if [[ "${#NESTED[@]}" -eq 0 ]]; then
    log "No nested clusters"
  else
    for name in "${NESTED[@]}"; do
      [[ -n "${name}" ]] || continue
      api_delete_nested "${name}"
    done
    if [[ "${DRY_RUN}" -eq 0 ]]; then
      sleep 10
      wait_delete_jobs || true
    fi
  fi
else
  log "Skipping cluster delete (--skip-cluster-delete)"
fi

# --- Phase 3: Ansible hub teardown ---
log "Phase 3: Ansible hub teardown (playbooks/teardown-hub.yml)"
ANSIBLE_EXTRA=(
  -e "teardown_hub_delete_harbor=true"
  -e "teardown_hub_delete_k3s=true"
  -e "teardown_hub_delete_wireguard=$([[ "${KEEP_WIREGUARD}" -eq 1 ]] && echo false || echo true)"
  -e "teardown_hub_delete_host_paths=$([[ "${KEEP_HOST_PATHS}" -eq 1 ]] && echo false || echo true)"
  -e "teardown_hub_delete_crds=$([[ "${KEEP_CRDS}" -eq 1 ]] && echo false || echo true)"
)

if [[ "${DRY_RUN}" -eq 1 ]]; then
  log "[dry-run] ansible-playbook ${INV} playbooks/teardown-hub.yml -l hub ${ANSIBLE_EXTRA[*]}"
else
  ansible-playbook ${INV} playbooks/teardown-hub.yml -l hub \
    "${VAULT_ARGS[@]}" \
    "${ANSIBLE_EXTRA[@]}"
fi

echo ""
log "Hub teardown finished."
echo ""
echo "Next steps for a fresh install from zero:"
echo "  1. Verify K3s is gone:  test ! -f /etc/rancher/k3s/k3s.yaml && echo OK"
echo "  2. Re-run hub install:   ./scripts/install-hub.sh"
echo "  3. Verify:               ./scripts/verify-hub-install.sh"
echo ""
echo "Inventory and vault under inventory/ are kept. Re-create workload sites and"
echo "clusters from the console after install."
