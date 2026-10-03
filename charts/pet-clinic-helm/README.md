# Pet Clinic Helm chart

This chart packages the Pet Clinic demonstration workload. Its templates define
the database, application, web, storage, and service resources.

## Render

From this directory:

```bash
helm template <release-name> .
```

Use an approved values file for deployment. Review the defaults and override
credentials, storage, image, resource, and ingress-related values for the
target environment.

The chart does not configure fleet routing by itself; expose a route with
`FleetApp` `route.mode` after install. Chart publication into Harbor is
managed outside this chart.

## References

- [Design guide](../../docs/design.md)
- [FleetApp Helm installs](../../docs/components/fleet-apps.md)
