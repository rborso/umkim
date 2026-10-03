#!/usr/bin/env bash
# Deep-merge Helm values YAML files (later files override). Writes one document to stdout.
#
#   scripts/render-llama-cpp-values.sh \
#     deploy/llama-cpp-helm/values-fleet-runtime.yaml \
#     deploy/llama-cpp-helm/profiles/qwen35-2b-ud-4g.yaml \
#     path/to/overlay-harbor.yaml
#
# Optional env for the example Harbor overlay (sed on a copied template):
#   HARBOR_REGISTRY=10.10.0.1:30443 GHCR_PROXY_PROJECT=ghcr-proxy \
#     scripts/render-llama-cpp-values.sh ... examples/llama-cpp-helm/overlay-harbor-ghcr-proxy.yaml.example

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 values1.yaml [values2.yaml ...]" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export RENDER_LLAMA_FILES=""
for f in "$@"; do
  if [[ ! -f "$f" ]]; then
    echo "missing file: $f" >&2
    exit 1
  fi
  RENDER_LLAMA_FILES="${RENDER_LLAMA_FILES}$(cd "$(dirname "$f")" && pwd)/$(basename "$f")|"
done

python3 - <<'PY'
import os, sys, yaml

def deep_merge(a, b):
    if not isinstance(a, dict) or not isinstance(b, dict):
        return b
    out = dict(a)
    for k, v in b.items():
        if k in out and isinstance(out[k], dict) and isinstance(v, dict):
            out[k] = deep_merge(out[k], v)
        else:
            out[k] = v
    return out

merged = {}
for path in os.environ["RENDER_LLAMA_FILES"].strip("|").split("|"):
    if not path:
        continue
    raw = open(path, encoding="utf-8").read()
    reg = os.environ.get("HARBOR_REGISTRY", "")
    proj = os.environ.get("GHCR_PROXY_PROJECT", "ghcr-proxy")
    if reg and "HARBOR_HOST" in raw:
        host, port = (reg.split(":", 1) + [""])[:2]
        raw = raw.replace("HARBOR_HOST", host).replace("HARBOR_HTTPS_PORT", port)
        raw = raw.replace("ghcr-proxy", proj)
    doc = yaml.safe_load(raw) or {}
    merged = deep_merge(merged, doc)

yaml.dump(merged, sys.stdout, default_flow_style=False, sort_keys=False)
PY
