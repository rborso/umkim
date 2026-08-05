import type { FieldHelpContent } from './fieldHelp';

/** Catalog item editor (Catalog page create/edit modal). */
export const CATALOG_EDITOR_FIELD_HELP: Record<string, FieldHelpContent> = {
  'catalog.displayName': {
    description: 'Human-readable catalog item title shown in the catalog list and deploy wizard.',
    example: 'Whoami Helm',
  },
  'catalog.serviceType': {
    description: 'DCM service type this template deploys: container, helm, operator, or cluster.',
    example: 'helm',
    examples: ['container', 'helm', 'operator', 'cluster'],
  },
  'catalog.description': {
    description: 'Short summary of the template for operators browsing the catalog.',
    example: 'Demo whoami chart with fleet edge routing fields',
  },
  'catalog.replicas': {
    description:
      'Default pod replicas on each placed cluster (workload.replicas). Not the number of clusters — that comes from placement policies.',
    example: '1',
  },
  'catalog.fieldPath': {
    description: 'Dot-path into the instance spec / values (e.g. routing.mode, metadata.name, values.replicaCount).',
    example: 'routing.mode',
  },
  'catalog.fieldDisplayName': {
    description: 'Label shown for this field in the deploy configure step.',
    example: 'Routing mode',
  },
  'catalog.fieldDescription': {
    description: 'Help text shown for this field at deploy time (overrides registry help when set).',
    example: 'How fleet edge exposes the app',
  },
  'catalog.fieldExample': {
    description: 'Example value shown in field help popovers.',
    example: 'hub',
  },
  'catalog.fieldDefault': {
    description: 'Default value pre-filled when deploying from this catalog item.',
    example: 'hub',
  },
  'catalog.fieldAllowedValues': {
    description: 'Comma-separated enum of allowed values (renders as a select when set).',
    example: 'hub, site, none',
  },
  'catalog.fieldUiTier': {
    description:
      'Where the field appears in the deploy wizard: required, common, or advanced. Leave empty to infer from the path.',
    example: 'common',
    examples: ['required', 'common', 'advanced'],
  },
  'catalog.fieldUiWidget': {
    description:
      'Optional UI widget override (text, textarea, select, switch, hostname, site_domain, etc.). Leave empty to infer.',
    example: 'select',
  },
  'helmImport.catalogId': {
    description: 'Stable catalog item id (DNS label). Used as ?id= on create and in deploy catalog_item_id.',
    example: 'nginx-helm',
  },
  'helmImport.repository': {
    description:
      'OCI chart repository prefix (oci://host[/path]). Examples: oci://registry-1.docker.io/bitnamicharts, oci://quay.io/org, oci://10.10.0.1:30443/charts. HTTPS Helm repos are not supported.',
    example: 'oci://registry-1.docker.io/bitnamicharts',
    examples: [
      'oci://registry-1.docker.io/bitnamicharts',
      'oci://quay.io/prometheus-community',
      'oci://10.10.0.1:30443/charts',
    ],
  },
  'helmImport.chartName': {
    description: 'Chart name under the OCI repository (helm upgrade uses repository/name).',
    example: 'nginx',
  },
  'helmImport.version': {
    description: 'Chart version / OCI tag the helm service provider will install.',
    example: '15.1.0',
  },
  'helmImport.values': {
    description: 'Optional default Helm values YAML stored on the catalog item (editable at deploy).',
    example: 'replicaCount: 1\n',
  },
};
