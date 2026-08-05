#!/usr/bin/env python3
"""Parse check-registry.yaml and list/filter checks for fleet-verify."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    yaml = None  # type: ignore


def load_registry(path: Path) -> list[dict]:
    text = path.read_text()
    if yaml is not None:
        data = yaml.safe_load(text)
    else:
        # Minimal fallback: expect JSON array in .yaml for dev without PyYAML
        data = json.loads(text)
    if not isinstance(data, list):
        raise SystemExit("registry must be a YAML list")
    return data


def check_matches(
    chk: dict,
    layer: int | None,
    suite: str | None,
    check_id: str | None,
    with_ansible: bool,
) -> bool:
    if check_id and chk.get("id") != check_id:
        return False
    layers = chk.get("layer") or []
    if layer is not None and layer not in layers:
        return False
    if suite and suite != "all":
        suites = chk.get("suite") or []
        if suite not in suites:
            return False
    if chk.get("id", "").startswith("ansible.") and not with_ansible:
        return False
    return True


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--registry", required=True)
    ap.add_argument("--layer", type=int)
    ap.add_argument("--suite", default="all")
    ap.add_argument("--id")
    ap.add_argument("--with-ansible", action="store_true")
    ap.add_argument("--format", choices=["json", "tsv"], default="json")
    args = ap.parse_args()

    checks = load_registry(Path(args.registry))
    out = [c for c in checks if check_matches(c, args.layer, args.suite, args.id, args.with_ansible)]

    if args.format == "json":
        print(json.dumps(out, indent=2))
    else:
        for c in out:
            layers = ",".join(str(x) for x in (c.get("layer") or []))
            suites = ",".join(c.get("suite") or [])
            profiles = ",".join(c.get("profiles") or [])
            handler = c.get("handler") or c.get("script") or ""
            print(
                "\t".join(
                    [
                        c.get("id", ""),
                        layers,
                        suites,
                        profiles,
                        c.get("mode", ""),
                        handler,
                        str(c.get("blocking", True)).lower(),
                        str(c.get("timeout_s", 300)),
                        ",".join(c.get("requires") or []),
                        str(c.get("parity", False)).lower(),
                    ]
                )
            )


if __name__ == "__main__":
    main()
