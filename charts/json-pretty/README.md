# JSON Pretty chart

This chart deploys a stateless, browser-side JSON formatter. The page assets,
deployment, service, and configuration are defined in this chart.

## Render

```bash
helm template <release-name> .
```

Review values and cluster policy before deployment. Chart publication,
ingress, and external access are managed outside this chart (Harbor +
FleetApp).

## References

- [FleetApp Helm installs](../../docs/components/fleet-apps.md)
- [Design guide](../../docs/design.md)
