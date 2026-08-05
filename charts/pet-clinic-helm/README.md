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

The chart does not configure fleet routing or catalog registration by itself;
those behaviors are controlled by the relevant deployment configuration.

## References

- [Design guide](../../docs/design.md)
- [DCM component](../../docs/components/dcm.md)
