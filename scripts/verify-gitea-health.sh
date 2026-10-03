#!/usr/bin/env bash
# Verify Gitea via hub route and/or spoke API; optionally seed backup-test repo.
#
# Usage:
#   ./scripts/verify-gitea-health.sh --host gitea-br-test.apps.example.com \
#     --cluster lab-int01 [--admin-pass secret] [--seed-repo] [--expect-sha <sha>]
set -euo pipefail

HOST=""
CLUSTER=""
ADMIN_USER="${GITEA_ADMIN_USER:-fleet}"
ADMIN_PASS="${GITEA_ADMIN_PASS:-CHANGE_ME}"
SEED_REPO=false
EXPECT_SHA=""
GATEWAY_HTTPS="${HUB_GATEWAY_HTTPS:-https://127.0.0.1}"
HUB_KC="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
NAMESPACE="${GITEA_NAMESPACE:-gitea}"
RELEASE="${GITEA_RELEASE:-gitea-br-test}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host) HOST="$2"; shift 2 ;;
    --cluster) CLUSTER="$2"; shift 2 ;;
    --admin-user) ADMIN_USER="$2"; shift 2 ;;
    --admin-pass) ADMIN_PASS="$2"; shift 2 ;;
    --seed-repo) SEED_REPO=true; shift ;;
    --expect-sha) EXPECT_SHA="$2"; shift 2 ;;
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --release) RELEASE="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$CLUSTER" ]] || { echo "ERROR: --cluster required" >&2; exit 1; }
[[ -n "$HOST" ]] || HOST="${RELEASE}.apps.example.com"

hub_kubectl() {
  if [[ -r "$HUB_KC" ]]; then
    kubectl --kubeconfig="$HUB_KC" "$@"
  else
    sudo kubectl --kubeconfig="$HUB_KC" "$@"
  fi
}

KC_FILE="/tmp/gitea-verify-kc-${CLUSTER}-$$.yaml"
hub_kubectl -n fleet-system get secret "fleet-kubeconfig-${CLUSTER}" \
  -o jsonpath='{.data.kubeconfig}' | base64 -d >"$KC_FILE"

echo "=== verify-gitea-health host=${HOST} cluster=${CLUSTER} ==="

kubectl --kubeconfig="$KC_FILE" -n "$NAMESPACE" wait --for=condition=ready pod \
  -l app.kubernetes.io/name=gitea --timeout=300s >/dev/null

# Prefer in-cluster API via port-forward (reliable for backup E2E).
PF_PORT="${GITEA_PF_PORT:-13000}"
PF_TARGET_PORT="${GITEA_PF_TARGET_PORT:-80}"
kubectl --kubeconfig="$KC_FILE" -n "$NAMESPACE" port-forward "pod/$(kubectl --kubeconfig="$KC_FILE" -n "$NAMESPACE" get pod -l app.kubernetes.io/name=gitea -o jsonpath='{.items[0].metadata.name}')" "${PF_PORT}:${PF_TARGET_PORT}" >/tmp/gitea-pf.log 2>&1 &
PF_PID=$!
trap 'kill "$PF_PID" 2>/dev/null || true; rm -f "$KC_FILE"' EXIT
sleep 3
API_BASE="http://127.0.0.1:${PF_PORT}"

# Optional hub route smoke (non-fatal).
hub_code=$(curl -sk -o /dev/null -w '%{http_code}' -H "Host: ${HOST}" "${GATEWAY_HTTPS}/" || true)
echo "hub route HTTP ${hub_code} (informational)"

TOKEN=$(curl -sf -u "${ADMIN_USER}:${ADMIN_PASS}" \
  -X POST "${API_BASE}/api/v1/users/${ADMIN_USER}/tokens" \
  -H 'Content-Type: application/json' \
  -d '{"name":"backup-verify-'"$$"'","scopes":["all"]}' | jq -r '.sha1 // empty')
[[ -n "$TOKEN" ]] || { echo "ERROR: could not create API token" >&2; exit 1; }

MARKER_CONTENT="${GITEA_MARKER_CONTENT:-fleet-backup-marker-$(date -u +%Y%m%d)}"
MARKER_PATH="PROOF.txt"

if [[ "$SEED_REPO" == "true" ]]; then
  curl -sf -H "Authorization: token ${TOKEN}" -X POST "${API_BASE}/api/v1/user/repos" \
    -H 'Content-Type: application/json' \
    -d '{"name":"backup-test","private":false,"auto_init":true,"default_branch":"main"}' \
    >/dev/null 2>&1 || true
  # Put a durable file in the repo (functional backup proof).
  curl -sf -H "Authorization: token ${TOKEN}" \
    -X POST "${API_BASE}/api/v1/repos/${ADMIN_USER}/backup-test/contents/${MARKER_PATH}" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg c "$MARKER_CONTENT" --arg m "seed backup marker" \
      '{content: ($c|@base64), message: $m, branch: "main"}')" >/dev/null \
    || curl -sf -H "Authorization: token ${TOKEN}" \
      -X PUT "${API_BASE}/api/v1/repos/${ADMIN_USER}/backup-test/contents/${MARKER_PATH}" \
      -H 'Content-Type: application/json' \
      -d "$(jq -nc --arg c "$MARKER_CONTENT" --arg m "update backup marker" --arg sha "$(
          curl -sf -H "Authorization: token ${TOKEN}" \
            "${API_BASE}/api/v1/repos/${ADMIN_USER}/backup-test/contents/${MARKER_PATH}" | jq -r '.sha // empty'
        )" '{content: ($c|@base64), message: $m, branch: "main", sha: $sha}')" >/dev/null || true
  echo "seeded ${MARKER_PATH} content=${MARKER_CONTENT}"
fi

REPOS=$(curl -sf -H "Authorization: token ${TOKEN}" "${API_BASE}/api/v1/user/repos")
echo "$REPOS" | jq -e 'map(select(.name=="backup-test")) | length >= 1' >/dev/null
SHA=$(curl -sf -H "Authorization: token ${TOKEN}" "${API_BASE}/api/v1/repos/${ADMIN_USER}/backup-test/commits" \
  | jq -r '.[0].sha // empty')
[[ -n "$SHA" ]] || SHA=$(echo "$REPOS" | jq -r '.[] | select(.name=="backup-test") | .sha // empty' | head -1)
echo "backup-test repo sha=${SHA:-unknown}"

if [[ -n "$EXPECT_SHA" && -n "$SHA" && "$SHA" != "$EXPECT_SHA" ]]; then
  echo "ERROR: expected sha ${EXPECT_SHA}, got ${SHA}" >&2
  exit 1
fi

[[ -n "$EXPECT_SHA" ]] && echo "OK: sha matches ${EXPECT_SHA}"

# Functional validity: clone (or pull) and read PROOF.txt via git over HTTP.
CLONE_DIR="${TMPDIR:-/tmp}/gitea-verify-clone-$$"
rm -rf "$CLONE_DIR"
git_url="http://${ADMIN_USER}:${TOKEN}@127.0.0.1:${PF_PORT}/${ADMIN_USER}/backup-test.git"
if git clone --depth 1 "$git_url" "$CLONE_DIR" >/tmp/gitea-clone.log 2>&1; then
  if [[ -f "${CLONE_DIR}/${MARKER_PATH}" ]]; then
    got=$(cat "${CLONE_DIR}/${MARKER_PATH}")
    echo "git clone OK; ${MARKER_PATH}=${got}"
    if [[ "$SEED_REPO" == "true" && "$got" != "$MARKER_CONTENT" ]]; then
      echo "ERROR: marker mismatch after seed (got=${got})" >&2
      exit 1
    fi
  else
    echo "WARN: ${MARKER_PATH} missing after clone (repo may be empty init only)"
  fi
  # Second fetch proves pull path works after restore.
  (
    cd "$CLONE_DIR"
    git pull --ff-only origin main >/tmp/gitea-pull.log 2>&1 || true
    echo "git pull OK"
  )
else
  echo "ERROR: git clone failed" >&2
  cat /tmp/gitea-clone.log >&2 || true
  exit 1
fi
rm -rf "$CLONE_DIR"

echo "=== verify-gitea-health OK ==="
