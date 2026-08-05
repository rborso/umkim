# Ebook Converter chart

This chart deploys a browser-based ebook conversion application from static
assets. Supported input handling is defined by the application files packaged
with the chart.

## Render

```bash
helm template <release-name> .
```

Review values, browser dependencies, and cluster policy before deployment.
Chart publication, catalog registration, ingress, and external access are
managed outside this chart.

## References

- [DCM component](../../docs/components/dcm.md)
- [Design guide](../../docs/design.md)
