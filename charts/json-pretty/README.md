# JSON Pretty chart

This chart deploys a stateless, browser-side JSON formatter. The page assets,
deployment, service, and configuration are defined in this chart.

## Render

```bash
helm template <release-name> .
```

Review values and cluster policy before deployment. Chart publication, catalog
registration, ingress, and external access are managed outside this chart.

## References

- [DCM component](../../docs/components/dcm.md)
- [Design guide](../../docs/design.md)
