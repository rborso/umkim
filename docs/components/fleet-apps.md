# FleetApp Helm installs

> Status: v0.2 is an experimental proof of concept. A successful Helm install
> is not proof that the application is healthy, reachable, or durable.

`FleetApp` installs an OCI Helm chart onto **one Ready cluster that you
select**. There is no placement engine, catalog scheduler, or DCM provider
path. The hub itself is not a valid target.

Publish a chart to the hub **library** once (`FleetChart` / Harbor `charts`),
then install it many times. Publishing is not an install.

## Library (publish)

Use the console **Apps → Library** tab, `umkim chart add` (see
[umkim-cli.md](umkim-cli.md)), `POST /fleet/api/v1alpha1/charts`, or
[scripts/fleet-chart.sh](../../scripts/fleet-chart.sh).

Sources:

- Upload a Helm `.tgz`
- An `oci://` URL on the Helm OCI allowlist
- An `http(s)://` URL to a `.tgz`

`imagePolicy` is `external` (spokes pull upstream images), `proxy` (metadata
only — expects Harbor proxy projects for upstream registries; the publish job
does not rewrite chart image references), or `copy` (best-effort `helm template`
+ `skopeo` into Harbor `mirror`, original chart name unchanged). Copied images
keep their upstream path under the `mirror` project; they are not renamed with
a `-local` suffix.

Seeded hub charts (`whoami`, `pet-clinic-helm`, `llama-cpp-helm`, `rocky-10-vm`)
appear in the library after Harbor publish and a Harbor import by
fleet-site-controller. For GPU inference, compose chart values from
`deploy/llama-cpp-helm/values-fleet-runtime.yaml` plus a file under
`profiles/` (see `scripts/render-llama-cpp-values.sh`) or use
`values-fleet-gpu-4g.yaml`; see [fleet-llm.md](fleet-llm.md).

Replacing a **Ready** chart with a different `origin` or `source` requires
`force=true` on `POST /charts` (JSON `force: true`) or `umkim chart add --force`.

Deleting a library row removes the `FleetChart` record only. Harbor artifacts
stay until an operator prunes them.

## Create (install)

Use the console **Apps** page, authenticated `POST /fleet/api/v1alpha1/apps`,
`umkim app create` (see [umkim-cli.md](umkim-cli.md)), or
[playbooks/flows/app-install.yml](../../playbooks/flows/app-install.yml).
The controller path is `/api/v1alpha1/apps`; `clusterName` is required.

Required fields:

| Field | Meaning |
|---|---|
| `clusterName` | Ready `FleetClusterJoin` / ManagedCluster name |
| `chart` | `oci://…` URL or a short name under the Harbor `charts` project |

Optional: `version`, `releaseName`, `namespace`, `values` / `valuesYaml`, and
`route.mode` (`none`, `hub`, `site`).

Console install fields adapt to the chart:

- **Built-in charts** show only applicable fields. `rocky-10-vm` exposes sizing,
  SSH keys, and network/static IP with no Route control; `llama-cpp-helm` exposes
  the profile picker and LiteLLM registration; `whoami` / `pet-clinic-helm`
  expose the fleet Route (`none`/`hub`/`site`) since they create a Service.
- **Custom charts** get the generic fields (cluster, chart, version, release,
  namespace, values) plus an optional Route. Selecting `hub`/`site` reveals the
  wiring (`serviceName`, `servicePort`, `serviceNamespace`, `healthPath`,
  `requireAuth`); the chart must create a matching Kubernetes Service.

### Update (patch)

`PATCH /fleet/api/v1alpha1/apps/{name}` updates `version`, `valuesYaml`,
`namespace`, and `route` on an existing install. The controller runs
`helm upgrade --install` when the spec hash changes. Returns **409** while
phase is `Installing` or `Deleting`. Console **Apps → Edit** and
`umkim app update` call this API.

### llama-cpp-helm profiles

`GET /llama-profiles` lists **FleetLlamaProfile** CRs; legacy
`GET /charts/llama-cpp-helm/profiles` returns the same ids for the install picker.
`GET /llama-profiles/{name}/values` (or the legacy chart path) returns merged Helm YAML (runtime +
profile + Harbor `ghcr-proxy` image). Use on install via console profile picker
or `umkim app create --llama-profile qwen35-2b-ud-4g`. Optional hub LiteLLM registration
uses `llmBackend` on `POST /apps` / `PATCH /apps/{name}` (console checkbox or
`umkim app create --llm-backend true|false`). Source YAML lives under
`deploy/llama-cpp-helm/` and is embedded in fleet-site-controller. See
[fleet-llama-profiles.md](fleet-llama-profiles.md) for CRUD, verify, and extended chat.

### Rocky Linux 10 guest (`rocky-10-vm`)

Install a KubeVirt VM from **Apps → Install chart** (chart `rocky-10-vm`) onto a
cluster that already has the KubeVirt platform. The console exposes **vCPU**,
**memory**, **disk**, **SSH public key(s)**, and **Network** (pod vs lan + static
IP) fields; they write Helm values. Use `route.mode: none`.

```yaml
cpu: 2
memory: 2Gi
disk:
  size: 20Gi
  storageClass: longhorn
  volumeMode: Filesystem
cloudInit:
  sshAuthorizedKeys:
    - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... you@host
```

The chart imports Rocky 10 GenericCloud via CDI (`volumeMode: Filesystem` is
required for Longhorn in this lab) and applies cloud-init (default serial
login `rocky` / `rocky`; add keys in **SSH public key(s)** or
`cloudInit.sshAuthorizedKeys`). For LAN guests, choose **Network = lan** (Multus
NAD, default `default/mgmt-l2`) and optionally set **Static IPv4**, **Gateway**,
and **DNS** — Rocky GenericCloud ignores cloud-init `network:` v2, so the chart
writes a NetworkManager keyfile instead. See [fleet-kubevirt.md](fleet-kubevirt.md)
and [charts/rocky-10-vm/README.md](../../charts/rocky-10-vm/README.md).

Short chart names expand to the hub Harbor prefix, for example `whoami` →
`oci://<hub-overlay-ip>:30443/charts/whoami`. Charts must match the
`helm_chart_oci_prefixes` allowlist (Harbor charts plus selected public
registries).

The cluster must already be **Ready**. Catalog and service-provider settings
are not used; install Helm with FleetApp.

## Routing

Per-app `route.mode` must match the site's exposure model. See
[fleet-edge.md](fleet-edge.md) and [../../examples/site-profiles/](../../examples/site-profiles/).

| Mode | Result | Hub required for public URL? |
|---|---|---|
| `none` | Chart-only; no `FleetAppRoute` | No |
| `hub` | Hub Traefik publishes `hubHost` (default `<release>.<hub edge domain>`) to this cluster's Service | Yes |
| `site` | Spoke ingress publishes `siteHost`; cluster must bind a `port_forward` or `reverse_proxy` ingress entry | No |

For `route.mode: site`, `siteHost` defaults to `<release>.<zone>` only when the
cluster is bound to a **`port_forward`** ingress (zone comes from that ingress row).
**`reverse_proxy`** clusters must set `siteHost` explicitly (no default).

Set `route.hubHost` or `route.siteHost` on create to override the default hostname. The console **Install chart** dialog exposes the same fields when route mode is `hub` or `site`.

`requireAuth` opts the route into ingress-auth when that stack is ready. See
[fleet-edge.md](fleet-edge.md) and [fleet-ingress-auth.md](../fleet-ingress-auth.md).

Hub-routed apps stop when the hub is unavailable. `site` and `none` apps can
remain reachable on their own paths when site ingress is healthy.

## Verify

After phase **Ready**, check the Helm release on the spoke (hub secret
`fleet-system/fleet-kubeconfig-<cluster>`). `scripts/verify-fleet-app.sh`
installs whoami and probes the pod when `FLEET_APP_CLUSTER` is set.

- For `hub` routes, confirm DNS and the externally reachable hub hostname.
- For `site` routes, confirm site DNS, port-forwards or reverse proxy, and the
  spoke Traefik path.

## Related

- [fleet-site.md](fleet-site.md) — cluster join until Ready
- [fleet-edge.md](fleet-edge.md) — site exposure and edge routing
- [harbor.md](harbor.md) — chart publishing
- [install.md](../install.md) — operator path
