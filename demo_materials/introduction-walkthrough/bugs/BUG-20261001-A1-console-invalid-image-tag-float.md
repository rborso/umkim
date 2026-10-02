# BUG-20261001-A1-console-invalid-image-tag-float

| Field | Content |
|-------|---------|
| **Stage** | A1 — install-hub / `fleet_console_helm` |
| **Environment** | umkim-hub greenfield 2026-10-01; `umkim_version: "0.2"` in common.yml |
| **Expected** | Helm values emit string tags `0.2`; pods pull `10.10.0.1:30443/fleet/umkim-console:0.2` |
| **Actual** | `roles/fleet_console_helm/templates/values.yaml.j2` renders unquoted `tag: 0.2`; Helm YAML parses float; kubelet sees `%!s(float64=0.2)` InvalidImageName |
| **Reproduce** | Install hub with numeric-looking version tag; wait on console Helm upgrade |
| **Evidence** | Pod describe; template lines `tag: {{ umkim_console_tag ...}}` without `quote` |
| **Workaround** | Quote tags in values.yaml.j2 (`{{ … \| quote }}`); patch Deployments to `:0.2` |
| **Follow-up** | Patch template in repo |

## Update
Ansible `| quote` is **shell** quoting and does not force YAML strings; unquoted `tag: 0.2` is still a float for Helm. Fix: `tag: "{{ ... }}"` in `values.yaml.j2`.
