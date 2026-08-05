import type { CatalogFieldDef, CatalogItemCreate } from '../api/types';

/** Default public OCI prefixes shown in import help (SP enforce via HELM_CHART_OCI_PREFIXES). */
export const DEFAULT_HELM_OCI_PREFIX_HINTS = [
  'oci://registry-1.docker.io',
  'oci://docker.io',
  'oci://quay.io',
  'oci://ghcr.io',
  'oci://<harbor-host>:30443/charts',
] as const;

export type HelmChartImportInput = {
  catalogId: string;
  displayName: string;
  description?: string;
  chartRepository: string;
  chartName: string;
  chartVersion: string;
  valuesYaml?: string;
};

/** dns-label: lowercase alphanumeric and hyphens, 1–63 chars. */
export function isDnsLabel(value: string): boolean {
  return /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(value);
}

export function suggestCatalogId(chartName: string): string {
  const base = chartName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 58);
  if (!base) return 'chart-helm';
  return base.endsWith('-helm') ? base : `${base}-helm`;
}

export function validateOciRepository(repo: string): string | null {
  const r = repo.trim();
  if (!r.startsWith('oci://')) {
    return 'OCI repository must start with oci://';
  }
  if (r === 'oci://' || r.length < 8) {
    return 'OCI repository is incomplete';
  }
  return null;
}

export function buildHelmImportFields(input: {
  chartRepository: string;
  chartName: string;
  chartVersion: string;
  valuesYaml?: string;
}): CatalogFieldDef[] {
  const valuesDefault = (input.valuesYaml ?? '').trim();
  return [
    {
      path: 'metadata.name',
      display_name: 'Helm release name',
      editable: true,
      required: true,
      validation_schema: {
        type: 'string',
        'x-ui-tier': 'required',
        'x-ui-widget': 'dns-label',
        'x-required': true,
      },
    },
    {
      path: 'chart.repository',
      display_name: 'Chart OCI repository',
      editable: false,
      default: input.chartRepository.trim().replace(/\/$/, ''),
      validation_schema: { type: 'string' },
    },
    {
      path: 'chart.name',
      display_name: 'Chart name',
      editable: false,
      default: input.chartName.trim(),
      validation_schema: { type: 'string' },
    },
    {
      path: 'chart.version',
      display_name: 'Chart version',
      editable: false,
      default: input.chartVersion.trim(),
      validation_schema: { type: 'string' },
    },
    {
      path: 'release.namespace',
      display_name: 'Target namespace',
      editable: true,
      default: 'default',
      validation_schema: {
        type: 'string',
        'x-ui-tier': 'common',
      },
    },
    {
      path: 'values',
      display_name: 'Helm values (full YAML)',
      editable: true,
      default: valuesDefault,
      validation_schema: {
        type: 'string',
        format: 'yaml',
        'x-ui-tier': 'advanced',
        'x-ui-widget': 'yaml',
      },
    },
    {
      path: 'install.wait',
      display_name: 'Wait for readiness',
      editable: true,
      default: true,
      validation_schema: {
        type: 'boolean',
        'x-ui-tier': 'advanced',
        'x-ui-widget': 'boolean',
      },
    },
    {
      path: 'install.timeout',
      display_name: 'Install timeout (seconds)',
      editable: true,
      default: 600,
      validation_schema: {
        type: 'integer',
        minimum: 60,
        'x-ui-tier': 'advanced',
        'x-ui-widget': 'number',
      },
    },
    {
      path: 'install.atomic',
      display_name: 'Atomic install',
      editable: true,
      default: true,
      validation_schema: {
        type: 'boolean',
        'x-ui-tier': 'advanced',
        'x-ui-widget': 'boolean',
      },
    },
    {
      path: 'routing.mode',
      display_name: 'Routing mode',
      editable: true,
      default: 'hub',
      validation_schema: {
        type: 'string',
        enum: ['hub', 'site', 'none'],
        'x-ui-tier': 'common',
      },
    },
    {
      path: 'routing.require_auth',
      display_name: 'Require Authelia (FleetAppRoute only)',
      editable: true,
      default: false,
      validation_schema: {
        type: 'boolean',
        'x-ui-widget': 'boolean',
        'x-ui-tier': 'common',
        'x-ui-depends-on': { path: 'routing.mode', in: ['hub', 'site'] },
      },
    },
    {
      path: 'routing.hub_host',
      display_name: 'Hub hostname (custom FQDN)',
      editable: true,
      default: '',
      description: 'Override hub URL. Leave empty for {release}.{hub-zone} from the fleet edge registry.',
      validation_schema: {
        type: 'string',
        'x-ui-widget': 'hostname',
        'x-ui-tier': 'common',
        'x-ui-depends-on': { path: 'routing.mode', in: ['hub'] },
      },
    },
    {
      path: 'routing.site_host',
      display_name: 'Site hostname (optional FQDN)',
      editable: true,
      default: '',
      validation_schema: {
        type: 'string',
        'x-ui-widget': 'hostname',
        'x-ui-tier': 'common',
        'x-ui-depends-on': { path: 'routing.mode', in: ['site'] },
      },
    },
    {
      path: 'routing.cluster',
      display_name: 'Target cluster (OCM name)',
      editable: true,
      default: '',
      description: 'Optional — pin to a cluster; otherwise policy selects from fleet.site + capabilities',
      validation_schema: {
        type: 'string',
        'x-ui-tier': 'advanced',
      },
    },
  ];
}

export function buildHelmImportCatalogItem(input: HelmChartImportInput): CatalogItemCreate & { id: string } {
  const fields = buildHelmImportFields({
    chartRepository: input.chartRepository,
    chartName: input.chartName,
    chartVersion: input.chartVersion,
    valuesYaml: input.valuesYaml,
  });
  return {
    id: input.catalogId.trim(),
    display_name: input.displayName.trim(),
    description: input.description?.trim() || undefined,
    service_type: 'helm',
    api_version: 'v1alpha1',
    spec: {
      service_type: 'helm',
      resources: [
        {
          name: 'release',
          service_type: 'helm',
          fields,
        },
      ],
    },
  };
}
