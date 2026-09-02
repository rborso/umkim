# Ebook Converter chart

This chart deploys a browser-based ebook conversion application from static
assets. Supported input handling is defined by the application files packaged
with the chart.

## Render

```bash
helm template <release-name> .
```

Review values, browser dependencies, and cluster policy before deployment.
Chart publication, ingress, and external access are managed outside this
chart (Harbor + FleetApp).

## References

- [FleetApp Helm installs](../../docs/components/fleet-apps.md)
- [Design guide](../../docs/design.md)
