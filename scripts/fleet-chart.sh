#!/usr/bin/env bash
# Publish, list, show, or delete hub library charts (FleetChart).
#
#   ./scripts/fleet-chart.sh list
#   ./scripts/fleet-chart.sh show whoami
#   ./scripts/fleet-chart.sh publish oci://ghcr.io/example/app --version 1.2.3
#   ./scripts/fleet-chart.sh publish ./charts/whoami
#   ./scripts/fleet-chart.sh publish ./whoami-0.1.0.tgz --images copy
#   ./scripts/fleet-chart.sh delete my-chart
#
# Identity headers match other fleet-flow scripts. Install remains
# playbooks/flows/app-install.yml / POST /apps.
set -euo pipefail

API="${FLEET_SITE_API:-http://127.0.0.1:18080/api/v1alpha1}"
HDR=(-H 'X-Fleet-Subject-Kind: User' -H 'X-Fleet-Subject-Name: fleet-flow' -H 'X-Fleet-DC-Admin: true')
IMAGES=external
VERSION=""
NAME=""

usage() {
  sed -n '2,14p' "$0" | sed 's/^# \?//'
  exit 2
}

cmd="${1:-}"
[[ -n "$cmd" ]] || usage
shift || true

args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --images) IMAGES="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --name) NAME="${2:-}"; shift 2 ;;
    --help|-h) usage ;;
    --) shift; args+=("$@"); break ;;
    -*) echo "unknown flag $1" >&2; usage ;;
    *) args+=("$1"); shift ;;
  esac
done

json_headers=("${HDR[@]}" -H 'Content-Type: application/json' -H 'Accept: application/json')

case "$cmd" in
  list)
    curl -sfS "${HDR[@]}" -H 'Accept: application/json' "${API}/charts" | python3 -m json.tool
    ;;
  show)
    n="${args[0]:-}"
    [[ -n "$n" ]] || { echo "show requires a chart name" >&2; exit 2; }
    curl -sfS "${HDR[@]}" -H 'Accept: application/json' "${API}/charts/${n}" | python3 -m json.tool
    ;;
  delete)
    n="${args[0]:-}"
    [[ -n "$n" ]] || { echo "delete requires a chart name" >&2; exit 2; }
    code=$(curl -sS -o /tmp/fleet-chart-delete.json -w '%{http_code}' "${HDR[@]}" -X DELETE "${API}/charts/${n}")
    if [[ "$code" != "202" && "$code" != "200" ]]; then
      echo "FAIL DELETE /charts/${n} HTTP ${code}" >&2
      cat /tmp/fleet-chart-delete.json >&2 || true
      exit 1
    fi
    echo "deleted ${n}"
    ;;
  publish)
    src="${args[0]:-}"
    [[ -n "$src" ]] || { echo "publish requires a path or URL" >&2; exit 2; }
    if [[ "$IMAGES" != "external" && "$IMAGES" != "proxy" && "$IMAGES" != "copy" ]]; then
      echo "--images must be external, proxy, or copy" >&2
      exit 2
    fi
    if [[ "$src" == oci://* || "$src" == http://* || "$src" == https://* ]]; then
      body=$(python3 - "$src" "$VERSION" "$IMAGES" "$NAME" <<'PY'
import json, sys
origin, version, images, name = sys.argv[1:5]
out = {"origin": origin, "imagePolicy": images}
if version:
    out["version"] = version
if name:
    out["name"] = name
print(json.dumps(out))
PY
)
      code=$(curl -sS -o /tmp/fleet-chart-publish.json -w '%{http_code}' "${json_headers[@]}" \
        -X POST "${API}/charts" -d "$body")
    else
      tgz="$src"
      if [[ -d "$src" ]]; then
        command -v helm >/dev/null 2>&1 || { echo "helm is required to package a chart directory" >&2; exit 1; }
        tmp=$(mktemp -d)
        helm package "$src" -d "$tmp" >/dev/null
        tgz=$(ls -1 "$tmp"/*.tgz | head -1)
      fi
      [[ -f "$tgz" ]] || { echo "not a file or directory: $src" >&2; exit 1; }
      form=(-F "chart=@${tgz}" -F "imagePolicy=${IMAGES}")
      [[ -n "$VERSION" ]] && form+=(-F "version=${VERSION}")
      [[ -n "$NAME" ]] && form+=(-F "name=${NAME}")
      code=$(curl -sS -o /tmp/fleet-chart-publish.json -w '%{http_code}' "${HDR[@]}" \
        -X POST "${API}/charts" "${form[@]}")
    fi
    if [[ "$code" != "201" && "$code" != "200" ]]; then
      echo "FAIL POST /charts HTTP ${code}" >&2
      cat /tmp/fleet-chart-publish.json >&2 || true
      exit 1
    fi
    python3 -m json.tool /tmp/fleet-chart-publish.json
    ;;
  *)
    usage
    ;;
esac
