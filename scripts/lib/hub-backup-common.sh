#!/usr/bin/env bash
# Shared helpers for hub backup and restore.
# shellcheck disable=SC2034
set -euo pipefail

hub_backup_lib_dir() {
  cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
}

hub_backup_repo_root() {
  local lib
  lib="$(hub_backup_lib_dir)"
  cd "${lib}/../.." && pwd
}

HUB_BACKUP_ROOT="${HUB_BACKUP_ROOT:-${BACKUP_ROOT:-/var/backups/umkim}}"
HUB_ACME_HOST_PATH="${HUB_ACME_HOST_PATH:-/var/lib/umkim/traefik-acme}"
HUB_INGRESS_NAMESPACE="${HUB_INGRESS_NAMESPACE:-fleet-ingress}"
HUB_INGRESS_GATEWAY="${HUB_INGRESS_GATEWAY:-gateway}"
HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
export KUBECONFIG

hub_log() {
  echo "[hub-backup] $*"
}

hub_warn() {
  echo "[hub-backup] WARN: $*" >&2
}

hub_die() {
  echo "[hub-backup] ERROR: $*" >&2
  exit 1
}

hub_require_root() {
  [[ "$(id -u)" -eq 0 ]] || hub_die "run as root (sudo)"
}

hub_backup_stamp() {
  date -u +%Y%m%dT%H%M%SZ
}

hub_umkim_version() {
  local root ver
  root="$(hub_backup_repo_root)"
  if [[ -f "${root}/VERSION" ]]; then
    tr -d '[:space:]' < "${root}/VERSION"
    return 0
  fi
  if command -v git >/dev/null 2>&1 && git -C "${root}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    ver="$(git -C "${root}" describe --tags --always --dirty 2>/dev/null || true)"
    [[ -n "${ver}" ]] && { echo "${ver}"; return 0; }
  fi
  echo "0.2.0"
}

hub_k3s_snap_dir() {
  echo "/var/lib/rancher/k3s/server/db/snapshots"
}

hub_k3s_bin() {
  if command -v k3s >/dev/null 2>&1; then
    command -v k3s
  elif [[ -x /usr/local/bin/k3s ]]; then
    echo /usr/local/bin/k3s
  else
    return 1
  fi
}

hub_kubectl() {
  command -v kubectl >/dev/null 2>&1 || return 1
  kubectl "$@"
}

hub_kubectl_ready() {
  hub_kubectl get --raw=/readyz >/dev/null 2>&1
}

hub_k3s_running() {
  systemctl is-active --quiet k3s 2>/dev/null
}

hub_k3s_datastore_type() {
  local k3s_bin err
  k3s_bin="$(hub_k3s_bin)" || { echo "none"; return 0; }
  if [[ -f /var/lib/rancher/k3s/server/db/state.db ]]; then
    err="$("${k3s_bin}" etcd-snapshot ls 2>&1 || true)"
    if [[ "${err}" == *"etcd datastore disabled"* ]]; then
      echo "sqlite"
      return 0
    fi
  fi
  echo "etcd"
}

hub_backup_sqlite() {
  local dest="$1"
  local dbdir="/var/lib/rancher/k3s/server/db"
  local tok="/var/lib/rancher/k3s/server/token"

  mkdir -p "${dest}/k3s-db"
  [[ -f "${dbdir}/state.db" ]] || hub_die "sqlite datastore missing: ${dbdir}/state.db"

  hub_log "k3s sqlite backup (state.db)"
  if command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "${dbdir}/state.db" ".backup '${dest}/k3s-db/state.db'"
  else
    hub_warn "sqlite3 not installed — copying state.db (stop k3s for a cold copy if consistency matters)"
    cp -a "${dbdir}/state.db" "${dest}/k3s-db/state.db"
  fi
  [[ -f "${tok}" ]] && cp -a "${tok}" "${dest}/k3s-db/token"
  echo "sqlite" > "${dest}/k3s-db/datastore.type"
  hub_log "sqlite backup: ${dest}/k3s-db/state.db"
}

hub_backup_k3s_datastore() {
  local dest="$1"
  local stamp="$2"

  case "$(hub_k3s_datastore_type)" in
    sqlite)
      hub_backup_sqlite "${dest}"
      ;;
    etcd)
      hub_backup_etcd "${dest}" "${stamp}"
      ;;
    *)
      hub_warn "k3s not found — skip cluster datastore"
      return 1
      ;;
  esac
}

hub_backup_etcd() {
  local dest="$1"
  local stamp="$2"
  local snap_dir snap_name latest k3s_bin

  mkdir -p "${dest}/etcd"
  k3s_bin="$(hub_k3s_bin)" || {
    hub_warn "k3s not found — skip etcd"
    return 1
  }

  snap_name="umkim-${stamp}"
  hub_log "k3s etcd snapshot (${snap_name})"
  "${k3s_bin}" etcd-snapshot save --name "${snap_name}"

  snap_dir="$(hub_k3s_snap_dir)"
  [[ -d "${snap_dir}" ]] || hub_die "snapshot dir missing: ${snap_dir}"

  latest="$(find "${snap_dir}" -maxdepth 1 -type f -name "${snap_name}*" | sort | tail -1)"
  [[ -n "${latest}" ]] || latest="$(find "${snap_dir}" -maxdepth 1 -type f -name '*.db' -printf '%T@ %p\n' 2>/dev/null | sort -n | tail -1 | cut -d' ' -f2-)"
  [[ -n "${latest}" && -f "${latest}" ]] || hub_die "no etcd snapshot produced in ${snap_dir}"

  cp -a "${latest}" "${dest}/etcd/"
  hub_log "etcd snapshot: ${dest}/etcd/$(basename "${latest}")"
}

hub_backup_wireguard() {
  local dest="$1"
  mkdir -p "${dest}/wireguard"
  [[ -d /etc/wireguard ]] || {
    hub_warn "/etc/wireguard missing — skip wireguard"
    return 1
  }
  tar -czf "${dest}/wireguard/wg.tgz" -C /etc/wireguard .
  hub_log "wireguard archive: ${dest}/wireguard/wg.tgz"
}

hub_backup_acme() {
  local dest="$1"
  local acme_path="${HUB_ACME_HOST_PATH}"
  mkdir -p "${dest}/host"
  if [[ ! -d "${acme_path}" ]]; then
    hub_warn "ACME host path missing (${acme_path}) — skip"
    return 1
  fi
  tar -czf "${dest}/host/traefik-acme.tgz" -C "${acme_path}" .
  hub_log "ACME archive: ${dest}/host/traefik-acme.tgz"
}

hub_harbor_pvc_map_json() {
  local ns="$1"
  local tmp
  tmp="$(mktemp)"
  hub_kubectl get pv -o json > "${tmp}"
  python3 - "${ns}" "${tmp}" <<'PY'
import json, sys

ns, path = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as fh:
    data = json.load(fh)
rows = []
for pv in data.get("items", []):
    ref = (pv.get("spec") or {}).get("claimRef") or {}
    if ref.get("namespace") != ns:
        continue
    host_path = None
    spec = pv.get("spec") or {}
    if "local" in spec:
        host_path = (spec["local"] or {}).get("path")
    elif "hostPath" in spec:
        host_path = (spec["hostPath"] or {}).get("path")
    if not host_path:
        continue
    rows.append({
        "namespace": ref.get("namespace"),
        "pvc": ref.get("name"),
        "pv": pv.get("metadata", {}).get("name"),
        "host_path": host_path,
    })
print(json.dumps(rows, indent=2))
PY
  rm -f "${tmp}"
}

hub_find_harbor_db_pod() {
  local ns="$1"
  local pod tmp
  tmp="$(mktemp)"
  hub_kubectl -n "${ns}" get pods -o json > "${tmp}" 2>/dev/null || { rm -f "${tmp}"; return 1; }
  pod="$(python3 - "${tmp}" <<'PY' || true
import json, sys
with open(sys.argv[1], encoding="utf-8") as fh:
    data = json.load(fh)
candidates = []
for pod in data.get("items", []):
    name = pod.get("metadata", {}).get("name", "")
    labels = pod.get("metadata", {}).get("labels") or {}
    if labels.get("app") == "harbor" and labels.get("component") == "database":
        candidates.append(name)
    elif name.endswith("-database-0"):
        candidates.append(name)
if candidates:
    print(sorted(candidates)[0])
PY
)"
  rm -f "${tmp}"
  [[ -n "${pod}" ]] && echo "${pod}"
}

hub_harbor_scale() {
  local ns="$1"
  local replicas="$2"
  local kind name

  hub_kubectl -n "${ns}" get deploy,sts -o name 2>/dev/null | while read -r kind; do
    hub_kubectl -n "${ns}" scale "${kind}" --replicas="${replicas}" >/dev/null 2>&1 || true
  done
  if [[ "${replicas}" -eq 0 ]]; then
    hub_log "Harbor scaled down in ${ns}"
  else
    hub_log "Harbor scaled up in ${ns}"
  fi
}

hub_backup_harbor() {
  local dest="$1"
  local ns="${HARBOR_NAMESPACE}"
  local db_pod pvc_map

  mkdir -p "${dest}/harbor/pvc"
  hub_kubectl_ready || {
    hub_warn "kubectl not ready — skip Harbor"
    return 1
  }
  hub_kubectl get namespace "${ns}" >/dev/null 2>&1 || {
    hub_warn "namespace ${ns} missing — skip Harbor"
    return 1
  }

  hub_log "Harbor backup (${ns}): quiesce workloads"
  hub_harbor_scale "${ns}" 0
  sleep 5

  pvc_map="$(hub_harbor_pvc_map_json "${ns}")"
  echo "${pvc_map}" > "${dest}/harbor/harbor-pvcs.json"

  python3 - "${dest}" "${pvc_map}" <<'PY'
import json, os, subprocess, sys

dest, raw = sys.argv[1], sys.argv[2]
rows = json.loads(raw)
pvc_dir = os.path.join(dest, "harbor", "pvc")
os.makedirs(pvc_dir, exist_ok=True)
for row in rows:
    claim = row["pvc"]
    path = row["host_path"]
    if not os.path.isdir(path):
        print(f"[hub-backup] WARN: missing host path for {claim}: {path}", file=sys.stderr)
        continue
    out = os.path.join(pvc_dir, f"{claim}.tgz")
    subprocess.check_call(["tar", "-czf", out, "-C", path, "."])
    print(f"[hub-backup] Harbor PVC {claim} -> {out}")
PY

  db_pod="$(hub_find_harbor_db_pod "${ns}")"
  if [[ -n "${db_pod}" ]]; then
    hub_log "Harbor database dump from pod ${db_pod}"
    if hub_kubectl -n "${ns}" exec "${db_pod}" -- sh -c 'command -v pg_dumpall >/dev/null && pg_dumpall -U postgres' \
      > "${dest}/harbor/registry.sql" 2>/dev/null; then
      hub_log "Harbor SQL dump: ${dest}/harbor/registry.sql"
    else
      hub_warn "pg_dumpall failed — PVC archives may still be sufficient"
      rm -f "${dest}/harbor/registry.sql"
    fi
  else
    hub_warn "Harbor database pod not found — PVC archives only"
  fi

  hub_harbor_scale "${ns}" 1
}

hub_write_manifest() {
  local dest="$1"
  shift
  local -a components=("$@")
  local stamp hostname version

  stamp="$(basename "${dest}")"
  hostname="$(hostname -f 2>/dev/null || hostname)"
  version="$(hub_umkim_version)"

  python3 - "${dest}/manifest.json" "${version}" "${stamp}" "${hostname}" "${HUB_ACME_HOST_PATH}" "${HARBOR_NAMESPACE}" \
    "${components[@]}" <<'PY'
import json, sys

path, version, stamp, hostname, acme_path, harbor_ns, *components = sys.argv[1:]
doc = {
    "schema": "umkim-hub-backup/v1",
    "umkim_version": version,
    "timestamp": stamp,
    "hostname": hostname,
    "components": sorted(set(components)),
    "paths": {
        "acme_host_path": acme_path,
        "harbor_namespace": harbor_ns,
        "wireguard": "/etc/wireguard",
        "k3s_snapshots": "/var/lib/rancher/k3s/server/db/snapshots",
    },
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(doc, fh, indent=2)
    fh.write("\n")
PY
}

hub_find_k3s_datastore_backup() {
  local src="$1"
  if [[ -f "${src}/k3s-db/state.db" ]]; then
    echo "${src}/k3s-db/state.db"
    return 0
  fi
  find "${src}/etcd" -maxdepth 1 -type f \( -name '*.db' -o -name '*snapshot*' \) 2>/dev/null | sort | tail -1
}

hub_k3s_datastore_backup_type() {
  local src="$1"
  if [[ -f "${src}/k3s-db/datastore.type" ]]; then
    tr -d '[:space:]' < "${src}/k3s-db/datastore.type"
    return 0
  fi
  if [[ -f "${src}/k3s-db/state.db" ]]; then
    echo "sqlite"
    return 0
  fi
  if find "${src}/etcd" -maxdepth 1 -type f \( -name '*.db' -o -name '*snapshot*' \) 2>/dev/null | grep -q .; then
    echo "etcd"
    return 0
  fi
  echo "none"
}

hub_restore_wireguard() {
  local src="$1"
  local archive="${src}/wireguard/wg.tgz"
  [[ -f "${archive}" ]] || hub_die "missing ${archive}"
  mkdir -p /etc/wireguard
  tar -xzf "${archive}" -C /etc/wireguard
  chmod 700 /etc/wireguard
  find /etc/wireguard -type f -name '*.conf' -exec chmod 600 {} +
  hub_log "restored WireGuard to /etc/wireguard"
  if systemctl list-unit-files wg-quick@.service >/dev/null 2>&1; then
    systemctl enable --now wg-quick@wg0 2>/dev/null || hub_warn "could not start wg-quick@wg0"
  fi
  local i
  for i in $(seq 1 15); do
    ip -4 addr show dev wg0 2>/dev/null | grep -q 'inet ' && return 0
    sleep 1
  done
  hub_warn "wg0 has no IPv4 address yet — k3s may fail to bind the API"
}

hub_restore_etcd() {
  local snap="$1"
  local k3s_bin
  [[ -f "${snap}" ]] || hub_die "etcd snapshot not found: ${snap}"
  k3s_bin="$(hub_k3s_bin)" || hub_die "k3s not installed"

  hub_log "stopping k3s"
  systemctl stop k3s 2>/dev/null || true
  sleep 2

  hub_log "restoring etcd from $(basename "${snap}")"
  "${k3s_bin}" server --cluster-reset --cluster-reset-restore-path="${snap}"

  hub_log "starting k3s"
  systemctl start k3s

  local i
  for i in $(seq 1 60); do
    if hub_kubectl_ready; then
      hub_log "k3s API ready"
      return 0
    fi
    sleep 5
  done
  hub_warn "k3s started but API not ready after 5 minutes — check journalctl -u k3s"
}

hub_restore_sqlite() {
  local src="$1"
  local archive="${src}/k3s-db/state.db"
  local token="${src}/k3s-db/token"
  local dbdir="/var/lib/rancher/k3s/server/db"

  [[ -f "${archive}" ]] || hub_die "sqlite backup missing: ${archive}"

  hub_log "stopping k3s"
  systemctl stop k3s 2>/dev/null || true
  sleep 2

  mkdir -p "${dbdir}"
  rm -f "${dbdir}/state.db" "${dbdir}/state.db-shm" "${dbdir}/state.db-wal"
  cp -a "${archive}" "${dbdir}/state.db"
  if [[ -f "${token}" ]]; then
    cp -a "${token}" /var/lib/rancher/k3s/server/token
  fi
  # Fresh k3s install may leave TLS newer than the restored DB; regenerate from datastore.
  rm -rf /var/lib/rancher/k3s/server/tls /var/lib/rancher/k3s/server/cred
  rm -f /etc/rancher/k3s/k3s.yaml

  hub_log "starting k3s"
  systemctl start k3s || hub_warn "k3s start failed — check wg0 and journalctl -u k3s"

  local i
  for i in $(seq 1 60); do
    if hub_kubectl_ready; then
      hub_log "k3s API ready"
      return 0
    fi
    sleep 5
  done
  hub_warn "k3s started but API not ready after 5 minutes — check journalctl -u k3s"
}

hub_restore_k3s_datastore() {
  local src="$1"
  local backup datastore

  backup="$(hub_find_k3s_datastore_backup "${src}")"
  [[ -n "${backup}" ]] || hub_die "no k3s datastore backup in ${src}"

  datastore="$(hub_k3s_datastore_backup_type "${src}")"
  case "${datastore}" in
    sqlite) hub_restore_sqlite "${src}" ;;
    etcd) hub_restore_etcd "${backup}" ;;
    *) hub_die "unknown datastore backup type in ${src}" ;;
  esac
}

hub_restore_acme() {
  local src="$1"
  local archive="${src}/host/traefik-acme.tgz"
  local acme_path="${HUB_ACME_HOST_PATH}"
  [[ -f "${archive}" ]] || {
    hub_warn "no ACME archive at ${archive} — skip"
    return 1
  }
  mkdir -p "${acme_path}"
  tar -xzf "${archive}" -C "${acme_path}"
  chmod 700 "${acme_path}"
  [[ -f "${acme_path}/acme.json" ]] && chmod 600 "${acme_path}/acme.json"
  hub_log "restored ACME data to ${acme_path}"
}

hub_restart_hub_gateway() {
  hub_kubectl_ready || return 0
  if hub_kubectl -n "${HUB_INGRESS_NAMESPACE}" get deployment "${HUB_INGRESS_GATEWAY}" >/dev/null 2>&1; then
    hub_kubectl -n "${HUB_INGRESS_NAMESPACE}" rollout restart "deployment/${HUB_INGRESS_GATEWAY}" >/dev/null
    hub_kubectl -n "${HUB_INGRESS_NAMESPACE}" rollout status "deployment/${HUB_INGRESS_GATEWAY}" --timeout=180s >/dev/null 2>&1 || true
    hub_log "restarted hub gateway (${HUB_INGRESS_NAMESPACE}/${HUB_INGRESS_GATEWAY})"
  fi
}

hub_restore_harbor() {
  local src="$1"
  local ns="${HARBOR_NAMESPACE}"
  local map_file="${src}/harbor/harbor-pvcs.json"
  local pvc_dir="${src}/harbor/pvc"

  [[ -f "${map_file}" ]] || {
    hub_warn "no Harbor PVC map — skip Harbor restore"
    return 1
  }
  hub_kubectl_ready || hub_die "kubectl not ready for Harbor restore"

  hub_log "Harbor restore (${ns}): scale down"
  hub_harbor_scale "${ns}" 0
  sleep 5

  python3 - "${map_file}" "${pvc_dir}" "${ns}" <<'PY'
import json, os, subprocess, sys, tempfile

map_file, pvc_dir, ns = sys.argv[1:4]
rows = json.load(open(map_file, encoding="utf-8"))

# Prefer live PV paths after etcd restore; fall back to backed-up paths.
live = {}
try:
    import subprocess as sp
    out = sp.check_output(["kubectl", "get", "pv", "-o", "json"], text=True)
    data = json.loads(out)
    for pv in data.get("items", []):
        ref = (pv.get("spec") or {}).get("claimRef") or {}
        if ref.get("namespace") != ns:
            continue
        path = None
        spec = pv.get("spec") or {}
        if "local" in spec:
            path = (spec["local"] or {}).get("path")
        elif "hostPath" in spec:
            path = (spec["hostPath"] or {}).get("path")
        if path:
            live[ref.get("name")] = path
except Exception as exc:
    print(f"[hub-backup] WARN: could not list live PV paths: {exc}", file=sys.stderr)

for row in rows:
    claim = row["pvc"]
    archive = os.path.join(pvc_dir, f"{claim}.tgz")
    if not os.path.isfile(archive):
        print(f"[hub-backup] WARN: missing archive for PVC {claim}", file=sys.stderr)
        continue
    dest = live.get(claim) or row.get("host_path")
    if not dest:
        print(f"[hub-backup] WARN: no restore path for PVC {claim}", file=sys.stderr)
        continue
    os.makedirs(dest, exist_ok=True)
    subprocess.check_call(["tar", "-xzf", archive, "-C", dest])
    print(f"[hub-backup] restored Harbor PVC {claim} -> {dest}")
PY

  sql="${src}/harbor/registry.sql"
  if [[ -f "${sql}" ]]; then
    local db_pod
    db_pod="$(hub_find_harbor_db_pod "${ns}")"
    if [[ -n "${db_pod}" ]]; then
      hub_harbor_scale "${ns}" 1
      sleep 15
      db_pod="$(hub_find_harbor_db_pod "${ns}")"
      if [[ -n "${db_pod}" ]] && hub_kubectl -n "${ns}" exec "${db_pod}" -- sh -c 'command -v psql >/dev/null'; then
        hub_warn "Harbor SQL dump present but auto-import skipped (PVC restore preferred); import manually if needed"
      fi
    fi
  fi

  hub_harbor_scale "${ns}" 1
  hub_log "Harbor workloads scaled up"
}

hub_backup_inventory_hint() {
  hub_log "Off-host: copy Ansible inventory + ansible-vault password (not included in this bundle)"
  hub_log "Off-host copy example: rsync -a <backup-dir>/ backup-host:umkim/"
}
