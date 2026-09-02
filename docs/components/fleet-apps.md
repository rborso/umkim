# FleetApp Helm installs

> Status: v0.2 is an experimental proof of concept. A successful Helm install
> is not proof that the application is healthy, reachable, or durable.

`FleetApp` installs an OCI Helm chart onto **one Ready cluster that you
select**. There is no placement engine, catalog scheduler, or DCM provider
path. The hub itself is not a valid target.

Publish a chart to the hub **library** once (`FleetChart` / Harbor `charts`),
then install it many times. Publishing is not an install.

## Library (publish)

Use the console **Apps → Library** tab, `POST /fleet/api/v1alpha1/charts`, or
[scripts/fleet-chart.sh](../../scripts/fleet-chart.sh).

Sources:

- Upload a Helm `.tgz`
- An `oci://` URL on the Helm OCI allowlist
- An `http(s)://` URL to a `.tgz`

`imagePolicy` is `external` (spokes pull upstream images), `proxy` (Harbor
pull-through for Quay/GHCR), or `copy` (best-effort `helm template` + `skopeo`
into Harbor `mirror`, original chart name unchanged). Copied images keep their
upstream path under the `mirror` project; they are not renamed with a `-local`
suffix.

Seeded hub charts (`whoami`, `pet-clinic-helm`, `llama-cpp-helm`) appear in the
library after Harbor publish and a Harbor import by fleet-site-controller.

Deleting a library row removes the `FleetChart` record only. Harbor artifacts
stay until an operator prunes them.

## Create (install)

Use the console **Apps** page, authenticated `POST /fleet/api/v1alpha1/apps`,
or [playbooks/flows/app-install.yml](../../playbooks/flows/app-install.yml).
The controller path is `/api/v1alpha1/apps`; `clusterName` is required.

Required fields:

| Field | Meaning |
|---|---|
| `clusterName` | Ready `FleetClusterJoin` / ManagedCluster name |
| `chart` | `oci://…` URL or a short name under the Harbor `charts` project |

Optional: `version`, `releaseName`, `namespace`, `values` / `valuesYaml`, and
`route.mode` (`none`, `hub`, `site`).

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
| `site` | Spoke ingress publishes `siteHost` (default `<release>.<site edge domain>`); requires `port_forward` or `reverse_proxy` site exposure | No |

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
